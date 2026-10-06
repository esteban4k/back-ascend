// Package journey holds the journey (history) use case.
package journey

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"ascend/internal/application/ports"
	"ascend/internal/domain/journey"
	"ascend/internal/domain/objective"
	"ascend/internal/domain/progression"
	"ascend/internal/domain/shared"
)

// Stats are lifetime totals.
type Stats struct {
	DaysActive          int     `json:"daysActive"`
	HabitCompletions    int     `json:"habitCompletions"`
	FocusHours          float64 `json:"focusHours"`
	ObjectivesCompleted int     `json:"objectivesCompleted"`
}

// Overview is the user's story so far.
type Overview struct {
	User       progression.User       `json:"user"`
	Level      progression.LevelState `json:"level"`
	Milestones []journey.Milestone    `json:"milestones"`
	Stats      Stats                  `json:"stats"`
}

// Service exposes the journey use case.
type Service struct{ p ports.Ports }

// NewService binds the journey use case to ports.
func NewService(p ports.Ports) *Service { return &Service{p: p} }

// Get returns milestones in chronological order plus lifetime stats.
func (s *Service) Get(ctx context.Context) (Overview, error) {
	today := s.p.Clock.Today()
	user, err := s.p.Users.GetCurrent(ctx)
	if err != nil {
		return Overview{}, err
	}
	milestones, err := s.p.Journey.GetAll(ctx)
	if err != nil {
		return Overview{}, err
	}
	habits, err := s.p.Habits.GetAll(ctx)
	if err != nil {
		return Overview{}, err
	}
	sessions, err := s.p.Focus.GetAll(ctx)
	if err != nil {
		return Overview{}, err
	}
	objectives, err := s.p.Objectives.GetAll(ctx)
	if err != nil {
		return Overview{}, err
	}

	slices.SortStableFunc(milestones, func(a, b journey.Milestone) int {
		return cmp.Or(strings.Compare(string(a.Date), string(b.Date)), cmp.Compare(a.Level, b.Level))
	})

	stats := Stats{DaysActive: shared.DaysBetween(ports.DateOf(s.p.Clock, user.JoinedAt), today) + 1}
	for _, h := range habits {
		stats.HabitCompletions += len(h.Completions)
	}
	seconds := 0
	for _, x := range sessions {
		seconds += x.FocusedSeconds
	}
	stats.FocusHours = float64(seconds) / 3600
	for _, o := range objectives {
		if o.Status == objective.Completed {
			stats.ObjectivesCompleted++
		}
	}
	return Overview{User: user, Level: user.Level(), Milestones: milestones, Stats: stats}, nil
}
