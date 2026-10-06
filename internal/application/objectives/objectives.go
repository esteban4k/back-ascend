// Package objectives holds the objective use cases.
package objectives

import (
	"cmp"
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
	"ascend/internal/domain/shared"
)

// LinkedHabit is a habit that feeds an objective.
type LinkedHabit struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Streak int    `json:"streak"`
}

// Summary is an objective plus its derived figures.
type Summary struct {
	Objective     objective.Objective `json:"objective"`
	Progress      float64             `json:"progress"`
	DaysRemaining int                 `json:"daysRemaining"`
	OnTrack       bool                `json:"onTrack"`
	Habits        []LinkedHabit       `json:"habits"`
}

// Service exposes the objective use cases.
type Service struct{ p ports.Ports }

// NewService binds the objective use cases to ports.
func NewService(p ports.Ports) *Service { return &Service{p: p} }

var statusOrder = map[objective.Status]int{objective.Active: 0, objective.Paused: 1, objective.Completed: 2}

func summarize(o objective.Objective, habits []habit.Habit, today shared.LocalDate) Summary {
	linked := []LinkedHabit{}
	for _, h := range habits {
		if slices.Contains(o.HabitIDs, h.ID) {
			linked = append(linked, LinkedHabit{ID: h.ID, Name: h.Name, Streak: habit.CurrentStreak(h, today)})
		}
	}
	return Summary{
		Objective:     o,
		Progress:      objective.Progress(o),
		DaysRemaining: objective.DaysRemaining(o, today),
		OnTrack:       objective.IsOnTrack(o, today),
		Habits:        linked,
	}
}

// List returns objectives, active first and then by deadline.
func (s *Service) List(ctx context.Context) ([]Summary, error) {
	today := s.p.Clock.Today()
	all, err := s.p.Objectives.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	habits, err := s.p.Habits.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	slices.SortStableFunc(all, func(a, b objective.Objective) int {
		return cmp.Or(cmp.Compare(statusOrder[a.Status], statusOrder[b.Status]), strings.Compare(string(a.Deadline), string(b.Deadline)))
	})
	out := make([]Summary, 0, len(all))
	for _, o := range all {
		out = append(out, summarize(o, habits, today))
	}
	return out, nil
}

// Get returns one objective with its summary.
func (s *Service) Get(ctx context.Context, id string) (Summary, error) {
	o, err := ports.Require(ctx, s.p.Objectives, "Objective", id)
	if err != nil {
		return Summary{}, err
	}
	habits, err := s.p.Habits.GetAll(ctx)
	if err != nil {
		return Summary{}, err
	}
	return summarize(o, habits, s.p.Clock.Today()), nil
}

// Current returns the active objective with the closest deadline: what the
// user is building right now. Nil when there is none.
func (s *Service) Current(ctx context.Context) (*Summary, error) {
	all, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, o := range all {
		if o.Objective.Status == objective.Active {
			return &o, nil
		}
	}
	return nil, nil
}

// Create adds an objective.
func (s *Service) Create(ctx context.Context, d objective.Draft) (objective.Objective, error) {
	return ports.InTx(ctx, s.p.Tx, func(ctx context.Context) (objective.Objective, error) {
		if err := objective.Validate(d); err != nil {
			return objective.Objective{}, err
		}
		if err := guard.Domain(ctx, s.p, d.DomainID); err != nil {
			return objective.Objective{}, err
		}
		d.HabitIDs = unique(d.HabitIDs)
		if err := guard.Habits(ctx, s.p, d.HabitIDs); err != nil {
			return objective.Objective{}, err
		}
		o, err := objective.New(s.p.IDs.Next(), d, s.p.Clock.Today(), s.p.IDs.Next)
		if err != nil {
			return objective.Objective{}, err
		}
		return o, s.p.Objectives.Save(ctx, o)
	})
}

func unique(ids []string) []string {
	out := []string{}
	for _, id := range ids {
		if id = strings.TrimSpace(id); id != "" && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

// registerProgress saves the change, grants the completion reward the first
// time the objective completes, and logs progress otherwise.
func (s *Service) registerProgress(ctx context.Context, before, after objective.Objective) (*progression.Reward, error) {
	if err := s.p.Objectives.Save(ctx, after); err != nil {
		return nil, err
	}
	delta := shared.RoundInt((objective.Progress(after) - objective.Progress(before)) * 100)

	if after.Status == objective.Completed && before.Status != objective.Completed {
		reward, err := progression.GrantReward(ctx, s.p, progression.RewardInput{
			XP:       objective.CompletionXP,
			DomainID: after.DomainID,
			RefID:    "objective:" + after.ID,
			Kind:     activity.KindObjective,
			Title:    "Objective completed · " + after.Name,
		})
		if err != nil {
			return nil, err
		}
		err = s.p.Journey.Save(ctx, journey.Milestone{
			ID:          "objective-" + after.ID,
			Level:       reward.Level.Level,
			Kind:        journey.KindObjective,
			Title:       after.Name,
			Description: "Objective completed",
			Date:        s.p.Clock.Today(),
		})
		if err != nil {
			return nil, err
		}
		return &reward, nil
	}

	if delta != 0 {
		sign := ""
		if delta > 0 {
			sign = "+"
		}
		err := s.p.Activity.Append(ctx, activity.Event{
			ID:       s.p.IDs.Next(),
			Kind:     activity.KindObjective,
			Title:    "Objective progressed",
			Detail:   fmt.Sprintf("%s%d%% · %s", sign, delta, after.Name),
			DomainID: after.DomainID,
			At:       s.p.Clock.Now().UTC(),
		})
		if err != nil {
			return nil, err
		}
	}
	return nil, nil
}

// RecordValue records a new metric measurement. Returns the completion
// reward when the target is reached for the first time, nil otherwise.
func (s *Service) RecordValue(ctx context.Context, id string, value float64) (*progression.Reward, error) {
	return ports.InTx(ctx, s.p.Tx, func(ctx context.Context) (*progression.Reward, error) {
		before, err := ports.Require(ctx, s.p.Objectives, "Objective", id)
		if err != nil {
			return nil, err
		}
		after, err := objective.RecordMetricValue(before, value, s.p.Clock.Today())
		if err != nil {
			return nil, err
		}
		return s.registerProgress(ctx, before, after)
	})
}

// ToggleMilestone flips a milestone. Returns the completion reward when the
// objective completes for the first time, nil otherwise.
func (s *Service) ToggleMilestone(ctx context.Context, id, milestoneID string) (*progression.Reward, error) {
	return ports.InTx(ctx, s.p.Tx, func(ctx context.Context) (*progression.Reward, error) {
		before, err := ports.Require(ctx, s.p.Objectives, "Objective", id)
		if err != nil {
			return nil, err
		}
		after, err := objective.ToggleMilestone(before, milestoneID, s.p.Clock.Today())
		if err != nil {
			return nil, err
		}
		return s.registerProgress(ctx, before, after)
	})
}

// SetStatus pauses, resumes or completes an objective by hand.
func (s *Service) SetStatus(ctx context.Context, id string, status objective.Status) (objective.Objective, error) {
	return ports.InTx(ctx, s.p.Tx, func(ctx context.Context) (objective.Objective, error) {
		current, err := ports.Require(ctx, s.p.Objectives, "Objective", id)
		if err != nil {
			return objective.Objective{}, err
		}
		o, err := objective.SetStatus(current, status, s.p.Clock.Today())
		if err != nil {
			return objective.Objective{}, err
		}
		return o, s.p.Objectives.Save(ctx, o)
	})
}

// Delete removes an objective.
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.p.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := ports.Require(ctx, s.p.Objectives, "Objective", id); err != nil {
			return err
		}
		return s.p.Objectives.Delete(ctx, id)
	})
}
