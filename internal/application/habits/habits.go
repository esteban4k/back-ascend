// Package habits holds the habit use cases.
package habits

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"ascend/internal/application/guard"
	"ascend/internal/application/ports"
	"ascend/internal/application/progression"
	"ascend/internal/domain/activity"
	"ascend/internal/domain/habit"
	"ascend/internal/domain/journey"
	"ascend/internal/domain/objective"
	domainprogression "ascend/internal/domain/progression"
	"ascend/internal/domain/shared"
)

type DayMark struct {
	Date shared.LocalDate `json:"date"`
	Due  bool             `json:"due"`
	Done bool             `json:"done"`
}

type Summary struct {
	Habit         habit.Habit     `json:"habit"`
	Streak        int             `json:"streak"`
	LongestStreak int             `json:"longestStreak"`
	Week          habit.WeekCount `json:"week"`
	Consistency   float64         `json:"consistency"`
	DueToday      bool            `json:"dueToday"`
	DoneToday     bool            `json:"doneToday"`
	// LastSeven covers the last 7 days, oldest first.
	LastSeven []DayMark `json:"lastSeven"`
}

type ConsistencyCell struct {
	Date   shared.LocalDate `json:"date"`
	Value  *float64         `json:"value"`
	Detail string           `json:"detail"`
}

type TriggeredReward struct {
	progression.Reward
	HabitName string `json:"habitName,omitempty"`
}

const MaxConsistencyDays = 731

type Service struct{ p ports.Ports }

func NewService(p ports.Ports) *Service { return &Service{p: p} }

func habitRef(habitID string, date shared.LocalDate) string {
	return fmt.Sprintf("habit:%s:%s", habitID, date)
}

func Summarize(h habit.Habit, today shared.LocalDate) Summary {
	lastSeven := make([]DayMark, 0, 7)
	for i := range 7 {
		date := today.AddDays(i - 6)
		lastSeven = append(lastSeven, DayMark{Date: date, Due: habit.IsDueOn(h, date), Done: habit.IsCompletedOn(h, date)})
	}
	return Summary{
		Habit:         h,
		Streak:        habit.CurrentStreak(h, today),
		LongestStreak: habit.LongestStreak(h, today),
		Week:          habit.WeekProgress(h, today),
		Consistency:   habit.Consistency(h, today, 30),
		DueToday:      habit.IsDueOn(h, today),
		DoneToday:     habit.IsCompletedOn(h, today),
		LastSeven:     lastSeven,
	}
}

func (s *Service) List(ctx context.Context) ([]Summary, error) {
	today := s.p.Clock.Today()
	all, err := s.p.Habits.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	active := slices.DeleteFunc(all, func(h habit.Habit) bool { return h.Archived })
	slices.SortStableFunc(active, func(a, b habit.Habit) int {
		return strings.Compare(string(a.CreatedAt), string(b.CreatedAt))
	})
	out := make([]Summary, 0, len(active))
	for _, h := range active {
		out = append(out, Summarize(h, today))
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, id string) (Summary, error) {
	h, err := ports.Require(ctx, s.p.Habits, "Habit", id)
	if err != nil {
		return Summary{}, err
	}
	return Summarize(h, s.p.Clock.Today()), nil
}

func (s *Service) Consistency(ctx context.Context, days int, habitID string) ([]ConsistencyCell, error) {
	if days < 1 || days > MaxConsistencyDays {
		return nil, shared.Invalidf("Days must be between 1 and %d", MaxConsistencyDays)
	}
	today := s.p.Clock.Today()
	all, err := s.p.Habits.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	if habitID != "" && !slices.ContainsFunc(all, func(h habit.Habit) bool { return h.ID == habitID }) {
		return nil, shared.NotFound("Habit", habitID)
	}
	habits := slices.DeleteFunc(all, func(h habit.Habit) bool {
		return h.Archived || (habitID != "" && h.ID != habitID)
	})

	cells := make([]ConsistencyCell, 0, days)
	for i := range days {
		date := today.AddDays(i - days + 1)
		due, done := 0, 0
		for _, h := range habits {
			if !habit.IsDueOn(h, date) {
				continue
			}
			due++
			if habit.IsCompletedOn(h, date) {
				done++
			}
		}
		if due == 0 {
			cells = append(cells, ConsistencyCell{Date: date, Detail: "Not scheduled"})
			continue
		}
		detail := fmt.Sprintf("%d/%d habits", done, due)
		if habitID != "" {
			switch {
			case done > 0:
				detail = "Completed"
			case date == today:
				detail = "Open"
			default:
				detail = "Missed"
			}
		}
		value := float64(done) / float64(due)
		cells = append(cells, ConsistencyCell{Date: date, Value: &value, Detail: detail})
	}
	return cells, nil
}

func (s *Service) checkDraft(ctx context.Context, d habit.Draft) error {
	if err := habit.Validate(d); err != nil {
		return err
	}
	return guard.Domain(ctx, s.p, d.DomainID)
}

func (s *Service) Create(ctx context.Context, d habit.Draft) (habit.Habit, error) {
	return ports.InTx(ctx, s.p.Tx, func(ctx context.Context) (habit.Habit, error) {
		if err := s.checkDraft(ctx, d); err != nil {
			return habit.Habit{}, err
		}
		h, err := habit.New(s.p.IDs.Next(), d, s.p.Clock.Today())
		if err != nil {
			return habit.Habit{}, err
		}
		return h, s.p.Habits.Save(ctx, h)
	})
}

func (s *Service) Update(ctx context.Context, id string, d habit.Draft) (habit.Habit, error) {
	return ports.InTx(ctx, s.p.Tx, func(ctx context.Context) (habit.Habit, error) {
		current, err := ports.Require(ctx, s.p.Habits, "Habit", id)
		if err != nil {
			return habit.Habit{}, err
		}
		if err := s.checkDraft(ctx, d); err != nil {
			return habit.Habit{}, err
		}
		h, err := habit.Update(current, d)
		if err != nil {
			return habit.Habit{}, err
		}
		return h, s.p.Habits.Save(ctx, h)
	})
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.p.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := ports.Require(ctx, s.p.Habits, "Habit", id); err != nil {
			return err
		}
		if err := s.p.Habits.Delete(ctx, id); err != nil {
			return err
		}
		objectives, err := s.p.Objectives.GetAll(ctx)
		if err != nil {
			return err
		}
		for _, o := range objectives {
			if slices.Contains(o.HabitIDs, id) {
				if err := s.p.Objectives.Save(ctx, objective.WithoutHabit(o, id)); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (s *Service) Complete(ctx context.Context, id string) (progression.Reward, error) {
	return ports.InTx(ctx, s.p.Tx, func(ctx context.Context) (progression.Reward, error) {
		return complete(ctx, s.p, id)
	})
}

func complete(ctx context.Context, p ports.Ports, id string) (progression.Reward, error) {
	today := p.Clock.Today()
	before, err := ports.Require(ctx, p.Habits, "Habit", id)
	if err != nil {
		return progression.Reward{}, err
	}
	h, err := habit.Complete(before, today)
	if err != nil {
		return progression.Reward{}, err
	}
	if err := p.Habits.Save(ctx, h); err != nil {
		return progression.Reward{}, err
	}

	previousStreak := habit.CurrentStreak(before, today)
	streak := habit.CurrentStreak(h, today)
	detail := ""
	if streak > 1 {
		detail = fmt.Sprintf("Streak × %d", streak)
	}
	reward, err := progression.GrantReward(ctx, p, progression.RewardInput{
		XP:       h.XP,
		DomainID: h.DomainID,
		RefID:    habitRef(id, today),
		Kind:     activity.KindHabit,
		Title:    "Completed " + h.Name,
		Detail:   detail,
	})
	if err != nil {
		return progression.Reward{}, err
	}

	if reached, ok := journey.StreakMilestoneReached(previousStreak, streak); ok {
		milestoneID := fmt.Sprintf("streak-%s-%d", id, reached)
		existing, err := p.Journey.GetByID(ctx, milestoneID)
		if err != nil {
			return progression.Reward{}, err
		}
		if existing == nil {
			err := p.Journey.Save(ctx, journey.Milestone{
				ID:          milestoneID,
				Level:       reward.Level.Level,
				Kind:        journey.KindStreak,
				Title:       fmt.Sprintf("%d× %s streak", reached, h.Name),
				Description: fmt.Sprintf("%s completed %d scheduled times in a row", h.Name, reached),
				Date:        today,
			})
			if err != nil {
				return progression.Reward{}, err
			}
		}
	}
	reward.Streak = &streak
	return reward, nil
}

func CompleteTriggered(ctx context.Context, p ports.Ports, trigger habit.Trigger) (*TriggeredReward, error) {
	today := p.Clock.Today()
	all, err := p.Habits.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(all, func(h habit.Habit) bool {
		return h.Trigger == trigger && habit.IsDueOn(h, today) && !habit.IsCompletedOn(h, today)
	})
	if i < 0 {
		return nil, nil
	}
	reward, err := complete(ctx, p, all[i].ID)
	if err != nil {
		return nil, err
	}
	return &TriggeredReward{Reward: reward, HabitName: all[i].Name}, nil
}

// Uncomplete un-checks today's completion and revokes its reward.
func (s *Service) Uncomplete(ctx context.Context, id string) (domainprogression.LevelState, error) {
	return ports.InTx(ctx, s.p.Tx, func(ctx context.Context) (domainprogression.LevelState, error) {
		today := s.p.Clock.Today()
		current, err := ports.Require(ctx, s.p.Habits, "Habit", id)
		if err != nil {
			return domainprogression.LevelState{}, err
		}
		h, err := habit.Uncomplete(current, today)
		if err != nil {
			return domainprogression.LevelState{}, err
		}
		if err := s.p.Habits.Save(ctx, h); err != nil {
			return domainprogression.LevelState{}, err
		}
		return progression.RevokeReward(ctx, s.p, habitRef(id, today))
	})
}
