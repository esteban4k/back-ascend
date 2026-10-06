// Package domains holds the life domain use cases.
package domains

import (
	"context"

	"ascend/internal/application/ports"
	"ascend/internal/domain/activity"
	"ascend/internal/domain/habit"
	"ascend/internal/domain/lifedomain"
	"ascend/internal/domain/objective"
	"ascend/internal/domain/shared"
)

// ObjectiveRef is an open objective of a domain.
type ObjectiveRef struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Progress float64 `json:"progress"`
}

// HabitRef is an active habit of a domain.
type HabitRef struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Frequency string `json:"frequency"`
	Streak    int    `json:"streak"`
}

// Summary is a life domain plus what feeds it.
type Summary struct {
	Domain lifedomain.LifeDomain `json:"domain"`
	Trend  lifedomain.Trend      `json:"trend"`
	// XP30d is the XP invested in the last 30 days.
	XP30d int `json:"xp30d"`
	// Share of all XP in the last 30 days.
	Share          float64          `json:"share"`
	Objectives     []ObjectiveRef   `json:"objectives"`
	Habits         []HabitRef       `json:"habits"`
	RecentActivity []activity.Event `json:"recentActivity"`
}

// Option is the lightweight shape used by pickers and labels.
type Option struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Service exposes the life domain use cases.
type Service struct{ p ports.Ports }

// NewService binds the life domain use cases to ports.
func NewService(p ports.Ports) *Service { return &Service{p: p} }

// List returns every domain with its figures for the last 30 days.
func (s *Service) List(ctx context.Context) ([]Summary, error) {
	today := s.p.Clock.Today()
	since := today.AddDays(-30).In(s.p.Clock.Location())
	domains, err := s.p.Domains.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	habits, err := s.p.Habits.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	objectives, err := s.p.Objectives.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	events, err := s.p.Activity.List(ctx, ports.ActivityQuery{Since: since})
	if err != nil {
		return nil, err
	}

	totalXP := 0
	for _, e := range events {
		totalXP += e.XP
	}
	if totalXP == 0 {
		totalXP = 1
	}

	out := make([]Summary, 0, len(domains))
	for _, d := range domains {
		sum := Summary{
			Domain:         d,
			Trend:          lifedomain.TrendOf(d),
			Objectives:     []ObjectiveRef{},
			Habits:         []HabitRef{},
			RecentActivity: []activity.Event{},
		}
		for _, e := range events {
			if e.DomainID != d.ID {
				continue
			}
			sum.XP30d += e.XP
			if len(sum.RecentActivity) < 5 {
				sum.RecentActivity = append(sum.RecentActivity, e)
			}
		}
		sum.Share = float64(sum.XP30d) / float64(totalXP)
		for _, o := range objectives {
			if o.DomainID == d.ID && o.Status != objective.Completed {
				sum.Objectives = append(sum.Objectives, ObjectiveRef{ID: o.ID, Name: o.Name, Progress: objective.Progress(o)})
			}
		}
		for _, h := range habits {
			if h.DomainID == d.ID && !h.Archived {
				sum.Habits = append(sum.Habits, HabitRef{
					ID: h.ID, Name: h.Name, Frequency: habit.FrequencyLabel(h.Schedule), Streak: habit.CurrentStreak(h, today),
				})
			}
		}
		out = append(out, sum)
	}
	return out, nil
}

// Options lists domains for pickers.
func (s *Service) Options(ctx context.Context) ([]Option, error) {
	domains, err := s.p.Domains.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Option, 0, len(domains))
	for _, d := range domains {
		out = append(out, Option{ID: d.ID, Name: d.Name})
	}
	return out, nil
}

// Create adds a life domain.
func (s *Service) Create(ctx context.Context, d lifedomain.Draft) (lifedomain.LifeDomain, error) {
	return ports.InTx(ctx, s.p.Tx, func(ctx context.Context) (lifedomain.LifeDomain, error) {
		created, err := lifedomain.New(s.p.IDs.Next(), d, lifedomain.InitialScore)
		if err != nil {
			return lifedomain.LifeDomain{}, err
		}
		return created, s.p.Domains.Save(ctx, created)
	})
}

// Update renames a life domain, keeping its score.
func (s *Service) Update(ctx context.Context, id string, d lifedomain.Draft) (lifedomain.LifeDomain, error) {
	return ports.InTx(ctx, s.p.Tx, func(ctx context.Context) (lifedomain.LifeDomain, error) {
		if err := lifedomain.Validate(d); err != nil {
			return lifedomain.LifeDomain{}, err
		}
		current, err := ports.Require(ctx, s.p.Domains, "Domain", id)
		if err != nil {
			return lifedomain.LifeDomain{}, err
		}
		updated, err := lifedomain.Rename(current, d)
		if err != nil {
			return lifedomain.LifeDomain{}, err
		}
		return updated, s.p.Domains.Save(ctx, updated)
	})
}

// Delete removes a domain that nothing depends on.
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.p.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := ports.Require(ctx, s.p.Domains, "Domain", id); err != nil {
			return err
		}
		habits, err := s.p.Habits.GetAll(ctx)
		if err != nil {
			return err
		}
		objectives, err := s.p.Objectives.GetAll(ctx)
		if err != nil {
			return err
		}
		for _, h := range habits {
			if h.DomainID == id && !h.Archived {
				return shared.Invalid("Move its habits and objectives to another domain first")
			}
		}
		for _, o := range objectives {
			if o.DomainID == id {
				return shared.Invalid("Move its habits and objectives to another domain first")
			}
		}
		return s.p.Domains.Delete(ctx, id)
	})
}
