// Package habit models recurring actions and their streaks.
package habit

import (
	"fmt"
	"slices"
	"strings"

	"ascend/internal/domain/shared"
)

type ScheduleKind string

const (
	Daily  ScheduleKind = "daily"
	Weekly ScheduleKind = "weekly"
)

type Schedule struct {
	Kind ScheduleKind `json:"kind"`

	Days []int `json:"days,omitempty"`
}

type Trigger string

const (
	TriggerJournal Trigger = "journal"
	TriggerWorkout Trigger = "workout"
)

type Habit struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	DomainID    string             `json:"domainId"`
	XP          int                `json:"xp"`
	Schedule    Schedule           `json:"schedule"`
	Time        string             `json:"time,omitempty"`
	Trigger     Trigger            `json:"trigger,omitempty"`
	Completions []shared.LocalDate `json:"completions"`
	CreatedAt   shared.LocalDate   `json:"createdAt"`
	Archived    bool               `json:"archived"`
}

type Draft struct {
	Name     string   `json:"name"`
	DomainID string   `json:"domainId"`
	XP       int      `json:"xp"`
	Schedule Schedule `json:"schedule"`
	Time     string   `json:"time,omitempty"`
	Trigger  Trigger  `json:"trigger,omitempty"`
}

var XPOptions = []int{20, 30, 40, 60, 80, 100, 150}

func Validate(d Draft) error {
	name := strings.TrimSpace(d.Name)
	switch {
	case name == "":
		return shared.Invalid("Give the habit a name")
	case len([]rune(name)) > 40:
		return shared.Invalid("Keep the name under 40 characters")
	case strings.TrimSpace(d.DomainID) == "":
		return shared.Invalid("Pick a domain")
	case d.XP <= 0 || d.XP > 500:
		return shared.Invalid("XP must be between 1 and 500")
	case d.Schedule.Kind != Daily && d.Schedule.Kind != Weekly:
		return shared.Invalid("Schedule must be daily or weekly")
	case d.Schedule.Kind == Weekly && len(d.Schedule.Days) == 0:
		return shared.Invalid("Pick at least one day")
	case d.Time != "" && !shared.IsClockTime(d.Time):
		return shared.Invalid("Time must be HH:mm")
	case d.Trigger != "" && d.Trigger != TriggerJournal && d.Trigger != TriggerWorkout:
		return shared.Invalid("Unknown trigger")
	}
	for _, day := range d.Schedule.Days {
		if day < 0 || day > 6 {
			return shared.Invalid("Days must be between 0 (Monday) and 6 (Sunday)")
		}
	}
	return nil
}

// New creates a habit from a draft.
func New(id string, d Draft, today shared.LocalDate) (Habit, error) {
	if err := Validate(d); err != nil {
		return Habit{}, err
	}
	return Habit{
		ID:          id,
		Name:        strings.TrimSpace(d.Name),
		DomainID:    d.DomainID,
		XP:          d.XP,
		Schedule:    normalizeSchedule(d.Schedule),
		Time:        d.Time,
		Trigger:     d.Trigger,
		Completions: []shared.LocalDate{},
		CreatedAt:   today,
	}, nil
}

// Update applies a draft to an existing habit, keeping its history.
func Update(h Habit, d Draft) (Habit, error) {
	if err := Validate(d); err != nil {
		return Habit{}, err
	}
	h.Name = strings.TrimSpace(d.Name)
	h.DomainID = d.DomainID
	h.XP = d.XP
	h.Schedule = normalizeSchedule(d.Schedule)
	h.Time = d.Time
	h.Trigger = d.Trigger
	return h, nil
}

func normalizeSchedule(s Schedule) Schedule {
	if s.Kind == Daily {
		return Schedule{Kind: Daily}
	}
	days := slices.Clone(s.Days)
	slices.Sort(days)
	days = slices.Compact(days)
	if len(days) == 7 {
		return Schedule{Kind: Daily}
	}
	return Schedule{Kind: Weekly, Days: days}
}

func FrequencyLabel(s Schedule) string {
	if s.Kind == Daily {
		return "Daily"
	}
	return fmt.Sprintf("%d× / week", len(s.Days))
}

func IsDueOn(h Habit, date shared.LocalDate) bool {
	if h.Archived || date < h.CreatedAt {
		return false
	}
	if h.Schedule.Kind == Daily {
		return true
	}
	return slices.Contains(h.Schedule.Days, date.Weekday())
}

func IsCompletedOn(h Habit, date shared.LocalDate) bool {
	return slices.Contains(h.Completions, date)
}

func Complete(h Habit, date shared.LocalDate) (Habit, error) {
	if IsCompletedOn(h, date) {
		return Habit{}, shared.Invalidf("%s is already complete", h.Name)
	}
	completions := append(slices.Clone(h.Completions), date)
	slices.Sort(completions)
	h.Completions = completions
	return h, nil
}

func Uncomplete(h Habit, date shared.LocalDate) (Habit, error) {
	if !IsCompletedOn(h, date) {
		return Habit{}, shared.Invalidf("%s is not complete", h.Name)
	}
	h.Completions = slices.DeleteFunc(slices.Clone(h.Completions), func(d shared.LocalDate) bool { return d == date })
	return h, nil
}

func CurrentStreak(h Habit, today shared.LocalDate) int {
	streak := 0
	date := today
	limit := shared.DaysBetween(h.CreatedAt, today)
	for i := 0; i <= limit; i, date = i+1, date.AddDays(-1) {
		if !IsDueOn(h, date) {
			continue
		}
		if IsCompletedOn(h, date) {
			streak++
		} else if date != today {
			break
		}
	}
	return streak
}

func LongestStreak(h Habit, today shared.LocalDate) int {
	best, run := 0, 0
	for d := h.CreatedAt; d <= today; d = d.AddDays(1) {
		if !IsDueOn(h, d) {
			continue
		}
		if IsCompletedOn(h, d) {
			run++
			best = max(best, run)
		} else if d != today {
			run = 0
		}
	}
	return best
}

type WeekCount struct {
	Done int `json:"done"`
	Due  int `json:"due"`
}

func WeekProgress(h Habit, date shared.LocalDate) WeekCount {
	monday := date.StartOfWeek()
	var c WeekCount
	for i := range 7 {
		d := monday.AddDays(i)
		if !IsDueOn(h, d) {
			continue
		}
		c.Due++
		if IsCompletedOn(h, d) {
			c.Done++
		}
	}
	return c
}

func Consistency(h Habit, today shared.LocalDate, days int) float64 {
	due, done := 0, 0
	for i := 1; i <= days; i++ {
		d := today.AddDays(-i)
		if !IsDueOn(h, d) {
			continue
		}
		due++
		if IsCompletedOn(h, d) {
			done++
		}
	}
	if due == 0 {
		return 0
	}
	return float64(done) / float64(due)
}
