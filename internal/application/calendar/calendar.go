// Package calendar merges everything scheduled or recorded into one agenda.
package calendar

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"ascend/internal/application/ports"
	"ascend/internal/domain/habit"
	"ascend/internal/domain/objective"
	"ascend/internal/domain/shared"
	"ascend/internal/domain/training"
)

// Kind classifies an agenda item.
type Kind string

const (
	KindTask      Kind = "task"
	KindHabit     Kind = "habit"
	KindFocus     Kind = "focus"
	KindObjective Kind = "objective"
	KindWorkout   Kind = "workout"
)

type Item struct {
	ID              string           `json:"id"`
	Kind            Kind             `json:"kind"`
	RefID           string           `json:"refId"`
	Title           string           `json:"title"`
	Date            shared.LocalDate `json:"date"`
	Start           string           `json:"start,omitempty"`
	DurationMinutes *int             `json:"durationMinutes,omitempty"`
	Done            bool             `json:"done"`
	Missed          bool             `json:"missed"`
	DomainID        string           `json:"domainId,omitempty"`
}

// MaxRangeDays bounds the agenda window.
const MaxRangeDays = 400

var defaultDuration = map[Kind]int{KindTask: 45, KindHabit: 30, KindFocus: 60, KindWorkout: 75}

// Service exposes the calendar use case.
type Service struct{ p ports.Ports }

// NewService binds the calendar use case to ports.
func NewService(p ports.Ports) *Service { return &Service{p: p} }

func minutes(v int) *int { return &v }

// Agenda returns everything between two dates (both included), sorted by date and start time.
func (s *Service) Agenda(ctx context.Context, from, to shared.LocalDate) ([]Item, error) {
	if !from.IsValid() || !to.IsValid() {
		return nil, shared.Invalid("Dates must be YYYY-MM-DD")
	}
	if to < from {
		return nil, shared.Invalid("The range must end after it starts")
	}
	if shared.DaysBetween(from, to) > MaxRangeDays {
		return nil, shared.Invalidf("The range cannot exceed %d days", MaxRangeDays)
	}

	today := s.p.Clock.Today()
	habits, err := s.p.Habits.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	tasks, err := s.p.Tasks.GetBetween(ctx, from, to)
	if err != nil {
		return nil, err
	}
	objectives, err := s.p.Objectives.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	sessions, err := s.p.Focus.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	workouts, err := s.p.Workouts.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	items := []Item{}
	for _, date := range shared.DateRange(from, to) {
		for _, h := range habits {
			if !habit.IsDueOn(h, date) {
				continue
			}
			done := habit.IsCompletedOn(h, date)
			items = append(items, Item{
				ID: "habit-" + h.ID + "-" + string(date), Kind: KindHabit, RefID: h.ID, Title: h.Name, Date: date,
				Start: h.Time, DurationMinutes: minutes(defaultDuration[KindHabit]), Done: done,
				Missed: !done && date < today, DomainID: h.DomainID,
			})
		}
	}

	for _, t := range tasks {
		duration := defaultDuration[KindTask]
		if t.DurationMinutes != nil {
			duration = *t.DurationMinutes
		}
		items = append(items, Item{
			ID: "task-" + t.ID, Kind: KindTask, RefID: t.ID, Title: t.Title, Date: t.Date, Start: t.Time,
			DurationMinutes: minutes(duration), Done: t.Done, DomainID: t.DomainID,
		})
	}

	for _, x := range sessions {
		started := x.StartedAt.In(s.p.Clock.Location())
		date := shared.DateOf(started)
		if !date.Within(from, to) {
			continue
		}
		items = append(items, Item{
			ID: "focus-" + x.ID, Kind: KindFocus, RefID: x.ID, Title: x.Label, Date: date, Start: started.Format("15:04"),
			DurationMinutes: minutes(max(15, shared.RoundInt(float64(x.FocusedSeconds)/60))), Done: true, DomainID: x.DomainID,
		})
	}

	for _, w := range workouts {
		if !w.Date.Within(from, to) {
			continue
		}
		duration := defaultDuration[KindWorkout]
		if w.DurationMinutes != nil {
			duration = *w.DurationMinutes
		}
		items = append(items, Item{
			ID: "workout-" + w.ID, Kind: KindWorkout, RefID: w.ID, Title: w.Name, Date: w.Date,
			DurationMinutes: minutes(duration), Done: w.Status == training.Completed, DomainID: training.DomainID,
		})
	}

	for _, o := range objectives {
		if !o.Deadline.Within(from, to) {
			continue
		}
		items = append(items, Item{
			ID: "objective-" + o.ID, Kind: KindObjective, RefID: o.ID, Title: "Deadline · " + o.Name, Date: o.Deadline,
			Done: o.Status == objective.Completed, DomainID: o.DomainID,
		})
	}

	slices.SortStableFunc(items, func(a, b Item) int {
		return cmp.Or(strings.Compare(string(a.Date), string(b.Date)), strings.Compare(a.Start, b.Start))
	})
	return items, nil
}
