// Package today holds the daily overview and the task use cases.
package today

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"ascend/internal/application/guard"
	"ascend/internal/application/ports"
	"ascend/internal/application/progression"
	"ascend/internal/domain/activity"
	"ascend/internal/domain/habit"
	domainprogression "ascend/internal/domain/progression"
	"ascend/internal/domain/shared"
	"ascend/internal/domain/task"
)

// Source says what a mission comes from.
type Source string

const (
	SourceHabit Source = "habit"
	SourceTask  Source = "task"
)

// Mission is a unit of action for the day: a scheduled habit or a one-off task.
type Mission struct {
	ID       string `json:"id"`
	Source   Source `json:"source"`
	Title    string `json:"title"`
	XP       int    `json:"xp"`
	DomainID string `json:"domainId"`
	Done     bool   `json:"done"`
	Time     string `json:"time,omitempty"`
	Streak   *int   `json:"streak,omitempty"`
}

// Overview is the day at a glance.
type Overview struct {
	Date      shared.LocalDate `json:"date"`
	Missions  []Mission        `json:"missions"`
	Completed int              `json:"completed"`
	Total     int              `json:"total"`
	// Progress is 0–1.
	Progress    float64 `json:"progress"`
	XPEarned    int     `json:"xpEarned"`
	XPAvailable int     `json:"xpAvailable"`
}

// Service exposes the today and task use cases.
type Service struct{ p ports.Ports }

// NewService binds the today use cases to ports.
func NewService(p ports.Ports) *Service { return &Service{p: p} }

func missionTime(m Mission) string {
	if m.Time == "" {
		return "99:99"
	}
	return m.Time
}

func byTime(a, b Mission) int {
	return cmp.Or(
		strings.Compare(missionTime(a), missionTime(b)),
		strings.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title)),
		strings.Compare(a.Title, b.Title),
	)
}

// Get builds today's missions from due habits and today's tasks.
func (s *Service) Get(ctx context.Context) (Overview, error) {
	date := s.p.Clock.Today()
	habits, err := s.p.Habits.GetAll(ctx)
	if err != nil {
		return Overview{}, err
	}
	tasks, err := s.p.Tasks.GetBetween(ctx, date, date)
	if err != nil {
		return Overview{}, err
	}

	missions := []Mission{}
	for _, h := range habits {
		if !habit.IsDueOn(h, date) {
			continue
		}
		streak := habit.CurrentStreak(h, date)
		missions = append(missions, Mission{
			ID: h.ID, Source: SourceHabit, Title: h.Name, XP: h.XP, DomainID: h.DomainID,
			Done: habit.IsCompletedOn(h, date), Time: h.Time, Streak: &streak,
		})
	}
	for _, t := range tasks {
		missions = append(missions, Mission{
			ID: t.ID, Source: SourceTask, Title: t.Title, XP: t.XP, DomainID: t.DomainID,
			Done: t.Done, Time: t.Time,
		})
	}
	slices.SortStableFunc(missions, byTime)

	o := Overview{Date: date, Missions: missions, Total: len(missions)}
	for _, m := range missions {
		o.XPAvailable += m.XP
		if m.Done {
			o.Completed++
			o.XPEarned += m.XP
		}
	}
	if o.Total > 0 {
		o.Progress = float64(o.Completed) / float64(o.Total)
	}
	return o, nil
}

func taskRef(id string) string { return "task:" + id }

// List returns tasks scheduled between two dates.
func (s *Service) List(ctx context.Context, from, to shared.LocalDate) ([]task.Task, error) {
	if !from.IsValid() || !to.IsValid() || to < from {
		return nil, shared.Invalid("Pick a valid date range")
	}
	tasks, err := s.p.Tasks.GetBetween(ctx, from, to)
	if err != nil {
		return nil, err
	}
	slices.SortStableFunc(tasks, func(a, b task.Task) int {
		return cmp.Or(strings.Compare(string(a.Date), string(b.Date)), strings.Compare(a.Time, b.Time))
	})
	return tasks, nil
}

// CreateTask schedules a one-off task.
func (s *Service) CreateTask(ctx context.Context, d task.Draft) (task.Task, error) {
	return ports.InTx(ctx, s.p.Tx, func(ctx context.Context) (task.Task, error) {
		if err := task.Validate(d); err != nil {
			return task.Task{}, err
		}
		if err := guard.Domain(ctx, s.p, d.DomainID); err != nil {
			return task.Task{}, err
		}
		if err := guard.Objective(ctx, s.p, d.ObjectiveID); err != nil {
			return task.Task{}, err
		}
		t, err := task.New(s.p.IDs.Next(), d)
		if err != nil {
			return task.Task{}, err
		}
		return t, s.p.Tasks.Save(ctx, t)
	})
}

// CompleteTask marks a task done and grants its reward.
func (s *Service) CompleteTask(ctx context.Context, id string) (progression.Reward, error) {
	return ports.InTx(ctx, s.p.Tx, func(ctx context.Context) (progression.Reward, error) {
		current, err := ports.Require(ctx, s.p.Tasks, "Task", id)
		if err != nil {
			return progression.Reward{}, err
		}
		t, err := task.Complete(current)
		if err != nil {
			return progression.Reward{}, err
		}
		if err := s.p.Tasks.Save(ctx, t); err != nil {
			return progression.Reward{}, err
		}
		return progression.GrantReward(ctx, s.p, progression.RewardInput{
			XP:       t.XP,
			DomainID: t.DomainID,
			RefID:    taskRef(id),
			Kind:     activity.KindTask,
			Title:    "Completed " + t.Title,
		})
	})
}

// ReopenTask marks a task open again and revokes its reward.
func (s *Service) ReopenTask(ctx context.Context, id string) (domainprogression.LevelState, error) {
	return ports.InTx(ctx, s.p.Tx, func(ctx context.Context) (domainprogression.LevelState, error) {
		current, err := ports.Require(ctx, s.p.Tasks, "Task", id)
		if err != nil {
			return domainprogression.LevelState{}, err
		}
		t, err := task.Reopen(current)
		if err != nil {
			return domainprogression.LevelState{}, err
		}
		if err := s.p.Tasks.Save(ctx, t); err != nil {
			return domainprogression.LevelState{}, err
		}
		return progression.RevokeReward(ctx, s.p, taskRef(id))
	})
}

// DeleteTask removes a task, revoking its reward if it was done.
func (s *Service) DeleteTask(ctx context.Context, id string) error {
	return s.p.Tx.WithinTx(ctx, func(ctx context.Context) error {
		t, err := ports.Require(ctx, s.p.Tasks, "Task", id)
		if err != nil {
			return err
		}
		if t.Done {
			if _, err := progression.RevokeReward(ctx, s.p, taskRef(id)); err != nil {
				return err
			}
		}
		return s.p.Tasks.Delete(ctx, id)
	})
}
