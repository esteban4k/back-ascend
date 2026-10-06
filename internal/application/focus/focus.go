// Package focus holds the deep-work use cases.
package focus

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"ascend/internal/application/guard"
	"ascend/internal/application/ports"
	"ascend/internal/application/progression"
	"ascend/internal/domain/activity"
	"ascend/internal/domain/focus"
	"ascend/internal/domain/journey"
	"ascend/internal/domain/shared"
)

// DaySeconds is focused time on one day.
type DaySeconds struct {
	Date    shared.LocalDate `json:"date"`
	Seconds int              `json:"seconds"`
}

// Overview summarizes focused time.
type Overview struct {
	TodaySeconds  int     `json:"todaySeconds"`
	WeekSeconds   int     `json:"weekSeconds"`
	SessionsToday int     `json:"sessionsToday"`
	TotalHours    float64 `json:"totalHours"`
	// LastSevenDays covers the last 7 days, oldest first.
	LastSevenDays []DaySeconds `json:"lastSevenDays"`
	// History holds the 30 most recent sessions, newest first.
	History []focus.Session `json:"history"`
}

// Completed is a recorded session plus its reward (nil when nothing was focused).
type Completed struct {
	Session focus.Session       `json:"session"`
	Reward  *progression.Reward `json:"reward"`
}

// Service exposes the focus use cases.
type Service struct{ p ports.Ports }

// NewService binds the focus use cases to ports.
func NewService(p ports.Ports) *Service { return &Service{p: p} }

func sum(sessions []focus.Session, keep func(focus.Session) bool) int {
	total := 0
	for _, s := range sessions {
		if keep == nil || keep(s) {
			total += s.FocusedSeconds
		}
	}
	return total
}

// Overview returns today's, this week's and lifetime focus figures.
func (s *Service) Overview(ctx context.Context) (Overview, error) {
	today := s.p.Clock.Today()
	monday := today.StartOfWeek()
	sessions, err := s.p.Focus.GetAll(ctx)
	if err != nil {
		return Overview{}, err
	}
	slices.SortStableFunc(sessions, func(a, b focus.Session) int { return cmp.Compare(b.StartedAt.UnixMilli(), a.StartedAt.UnixMilli()) })
	date := func(x focus.Session) shared.LocalDate { return ports.DateOf(s.p.Clock, x.StartedAt) }

	o := Overview{
		TodaySeconds: sum(sessions, func(x focus.Session) bool { return date(x) == today }),
		WeekSeconds:  sum(sessions, func(x focus.Session) bool { return date(x) >= monday }),
		TotalHours:   float64(sum(sessions, nil)) / 3600,
		History:      sessions[:min(30, len(sessions))],
	}
	for _, x := range sessions {
		if date(x) == today {
			o.SessionsToday++
		}
	}
	o.LastSevenDays = make([]DaySeconds, 0, 7)
	for i := range 7 {
		d := today.AddDays(i - 6)
		o.LastSevenDays = append(o.LastSevenDays, DaySeconds{Date: d, Seconds: sum(sessions, func(x focus.Session) bool { return date(x) == d })})
	}
	return o, nil
}

// Start starts a timer. The running timer lives with the client until it is
// finished, so nothing is stored yet.
func (s *Service) Start(ctx context.Context, in focus.TimerInput) (focus.Timer, error) {
	if err := guard.Domain(ctx, s.p, in.DomainID); err != nil {
		return focus.Timer{}, err
	}
	return focus.StartTimer(in, s.p.Clock.Now().UnixMilli())
}

// Complete records the session, grants XP for the focused time and records
// cumulative focus milestones in the journey.
func (s *Service) Complete(ctx context.Context, timer focus.Timer) (Completed, error) {
	return ports.InTx(ctx, s.p.Tx, func(ctx context.Context) (Completed, error) {
		now := s.p.Clock.Now().UnixMilli()
		if err := focus.ValidateTimer(timer, now); err != nil {
			return Completed{}, err
		}
		if err := guard.Domain(ctx, s.p, timer.DomainID); err != nil {
			return Completed{}, err
		}
		all, err := s.p.Focus.GetAll(ctx)
		if err != nil {
			return Completed{}, err
		}
		previousHours := float64(sum(all, nil)) / 3600

		session := focus.Finish(s.p.IDs.Next(), timer, now)
		if err := s.p.Focus.Save(ctx, session); err != nil {
			return Completed{}, err
		}

		minutes := shared.RoundInt(float64(session.FocusedSeconds) / 60)
		if minutes == 0 {
			return Completed{Session: session}, nil
		}
		reward, err := progression.GrantReward(ctx, s.p, progression.RewardInput{
			XP:       session.XP,
			DomainID: session.DomainID,
			RefID:    "focus:" + session.ID,
			Kind:     activity.KindFocus,
			Title:    "Deep work · " + session.Label,
			Detail:   fmt.Sprintf("%d min focused", minutes),
		})
		if err != nil {
			return Completed{}, err
		}

		hours := previousHours + float64(session.FocusedSeconds)/3600
		if crossed, ok := journey.FocusMilestoneReached(previousHours, hours); ok {
			err := s.p.Journey.Save(ctx, journey.Milestone{
				ID:          fmt.Sprintf("focus-%g", crossed),
				Level:       reward.Level.Level,
				Kind:        journey.KindFocus,
				Title:       fmt.Sprintf("%g hours focused", crossed),
				Description: "Cumulative deep work across all sessions",
				Date:        s.p.Clock.Today(),
			})
			if err != nil {
				return Completed{}, err
			}
		}
		return Completed{Session: session, Reward: &reward}, nil
	})
}
