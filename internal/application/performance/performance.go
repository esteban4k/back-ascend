// Package performance builds the analytics report.
package performance

import (
	"cmp"
	"context"
	"slices"

	"ascend/internal/application/ports"
	"ascend/internal/domain/activity"
	"ascend/internal/domain/focus"
	"ascend/internal/domain/habit"
	"ascend/internal/domain/objective"
	"ascend/internal/domain/shared"
)

// KPI compares the last 30 days with the 30 before.
type KPI struct {
	Current  float64 `json:"current"`
	Previous float64 `json:"previous"`
}

// KPIs are the headline figures.
type KPIs struct {
	XP              KPI `json:"xp"`
	HabitsCompleted KPI `json:"habitsCompleted"`
	Consistency     KPI `json:"consistency"`
	FocusHours      KPI `json:"focusHours"`
}

// DailyXP is XP earned on one day.
type DailyXP struct {
	Date shared.LocalDate `json:"date"`
	XP   int              `json:"xp"`
}

// WeekdayXP is cumulative XP by weekday. ThisWeek is nil for days still ahead.
type WeekdayXP struct {
	Day      string `json:"day"`
	ThisWeek *int   `json:"thisWeek"`
	LastWeek int    `json:"lastWeek"`
}

// WeekRate is the habit completion rate of one ISO week.
type WeekRate struct {
	Week shared.LocalDate `json:"week"`
	Rate float64          `json:"rate"`
}

// WeekHours is focused hours in one ISO week.
type WeekHours struct {
	Week  shared.LocalDate `json:"week"`
	Hours float64          `json:"hours"`
}

// MonthXP is XP earned in one month.
type MonthXP struct {
	Month shared.LocalDate `json:"month"`
	XP    int              `json:"xp"`
}

// DomainXP is XP invested in one domain over the last 30 days.
type DomainXP struct {
	DomainID string `json:"domainId"`
	Name     string `json:"name"`
	XP       int    `json:"xp"`
}

// StreakRow is a habit's current and best streak.
type StreakRow struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Streak  int    `json:"streak"`
	Longest int    `json:"longest"`
}

// ObjectiveRow is an active objective's pace.
type ObjectiveRow struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Progress float64 `json:"progress"`
	OnTrack  bool    `json:"onTrack"`
}

// Report is everything the performance screen charts.
type Report struct {
	KPIs              KPIs           `json:"kpis"`
	DailyXP           []DailyXP      `json:"dailyXp"`
	WeekComparison    []WeekdayXP    `json:"weekComparison"`
	WeeklyConsistency []WeekRate     `json:"weeklyConsistency"`
	WeeklyFocusHours  []WeekHours    `json:"weeklyFocusHours"`
	MonthlyXP         []MonthXP      `json:"monthlyXp"`
	XPByDomain        []DomainXP     `json:"xpByDomain"`
	Streaks           []StreakRow    `json:"streaks"`
	Objectives        []ObjectiveRow `json:"objectives"`
}

var weekdays = []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}

// Service exposes the performance use case.
type Service struct{ p ports.Ports }

// NewService binds the performance use case to ports.
func NewService(p ports.Ports) *Service { return &Service{p: p} }

type habitStats struct {
	completions int
	rate        float64
}

func statsBetween(habits []habit.Habit, from, to shared.LocalDate) habitStats {
	due, done, completions := 0, 0, 0
	for _, date := range shared.DateRange(from, to) {
		for _, h := range habits {
			completed := habit.IsCompletedOn(h, date)
			if completed {
				completions++
			}
			if !habit.IsDueOn(h, date) {
				continue
			}
			due++
			if completed {
				done++
			}
		}
	}
	s := habitStats{completions: completions}
	if due > 0 {
		s.rate = float64(done) / float64(due)
	}
	return s
}

// Report builds the analytics for the last 30 days, 12 weeks and 6 months.
func (s *Service) Report(ctx context.Context) (Report, error) {
	today := s.p.Clock.Today()
	events, err := s.p.Activity.List(ctx, ports.ActivityQuery{})
	if err != nil {
		return Report{}, err
	}
	habits, err := s.p.Habits.GetAll(ctx)
	if err != nil {
		return Report{}, err
	}
	sessions, err := s.p.Focus.GetAll(ctx)
	if err != nil {
		return Report{}, err
	}
	domains, err := s.p.Domains.GetAll(ctx)
	if err != nil {
		return Report{}, err
	}
	objectives, err := s.p.Objectives.GetAll(ctx)
	if err != nil {
		return Report{}, err
	}

	eventDate := func(e activity.Event) shared.LocalDate { return ports.DateOf(s.p.Clock, e.At) }
	sessionDate := func(x focus.Session) shared.LocalDate { return ports.DateOf(s.p.Clock, x.StartedAt) }

	xpByDate := map[shared.LocalDate]int{}
	for _, e := range events {
		xpByDate[eventDate(e)] += e.XP
	}
	xpBetween := func(from, to shared.LocalDate) int {
		total := 0
		for d, xp := range xpByDate {
			if d.Within(from, to) {
				total += xp
			}
		}
		return total
	}
	focusHoursBetween := func(from, to shared.LocalDate) float64 {
		seconds := 0
		for _, x := range sessions {
			if sessionDate(x).Within(from, to) {
				seconds += x.FocusedSeconds
			}
		}
		return float64(seconds) / 3600
	}

	from := today.AddDays(-29)
	prevFrom := today.AddDays(-59)
	prevTo := today.AddDays(-30)
	// Consistency excludes today, which is still in progress.
	current := statsBetween(habits, today.AddDays(-30), today.AddDays(-1))
	previous := statsBetween(habits, today.AddDays(-60), today.AddDays(-31))

	r := Report{
		KPIs: KPIs{
			XP:              KPI{Current: float64(xpBetween(from, today)), Previous: float64(xpBetween(prevFrom, prevTo))},
			HabitsCompleted: KPI{Current: float64(current.completions), Previous: float64(previous.completions)},
			Consistency:     KPI{Current: current.rate, Previous: previous.rate},
			FocusHours:      KPI{Current: focusHoursBetween(from, today), Previous: focusHoursBetween(prevFrom, prevTo)},
		},
	}

	r.DailyXP = []DailyXP{}
	for _, date := range shared.DateRange(from, today) {
		r.DailyXP = append(r.DailyXP, DailyXP{Date: date, XP: xpByDate[date]})
	}

	thisMonday := today.StartOfWeek()
	lastMonday := thisMonday.AddDays(-7)
	cumThis, cumLast := 0, 0
	r.WeekComparison = make([]WeekdayXP, 0, 7)
	for i, day := range weekdays {
		d := thisMonday.AddDays(i)
		cumLast += xpByDate[lastMonday.AddDays(i)]
		cumThis += xpByDate[d]
		row := WeekdayXP{Day: day, LastWeek: cumLast}
		if d <= today {
			v := cumThis
			row.ThisWeek = &v
		}
		r.WeekComparison = append(r.WeekComparison, row)
	}

	yesterday := today.AddDays(-1)
	r.WeeklyConsistency = make([]WeekRate, 0, 12)
	r.WeeklyFocusHours = make([]WeekHours, 0, 12)
	for i := range 12 {
		week := thisMonday.AddDays((i - 11) * 7)
		end := shared.MinDate(week.AddDays(6), yesterday)
		rate := 0.0
		if end >= week {
			rate = statsBetween(habits, week, end).rate
		}
		r.WeeklyConsistency = append(r.WeeklyConsistency, WeekRate{Week: week, Rate: rate})
		r.WeeklyFocusHours = append(r.WeeklyFocusHours, WeekHours{Week: week, Hours: focusHoursBetween(week, week.AddDays(6))})
	}

	monthStart := today.StartOfMonth()
	months := make([]shared.LocalDate, 6)
	for i := range 6 {
		months[i] = monthStart.AddMonths(i - 5)
	}
	r.MonthlyXP = make([]MonthXP, 0, 6)
	for i, month := range months {
		next := today.AddDays(1)
		if i+1 < len(months) {
			next = months[i+1]
		}
		r.MonthlyXP = append(r.MonthlyXP, MonthXP{Month: month, XP: xpBetween(month, next.AddDays(-1))})
	}

	r.XPByDomain = make([]DomainXP, 0, len(domains))
	for _, d := range domains {
		xp := 0
		for _, e := range events {
			if e.DomainID == d.ID && eventDate(e) >= from {
				xp += e.XP
			}
		}
		r.XPByDomain = append(r.XPByDomain, DomainXP{DomainID: d.ID, Name: d.Name, XP: xp})
	}
	slices.SortStableFunc(r.XPByDomain, func(a, b DomainXP) int { return cmp.Compare(b.XP, a.XP) })

	r.Streaks = []StreakRow{}
	for _, h := range habits {
		if !h.Archived {
			r.Streaks = append(r.Streaks, StreakRow{ID: h.ID, Name: h.Name, Streak: habit.CurrentStreak(h, today), Longest: habit.LongestStreak(h, today)})
		}
	}
	slices.SortStableFunc(r.Streaks, func(a, b StreakRow) int { return cmp.Compare(b.Streak, a.Streak) })

	r.Objectives = []ObjectiveRow{}
	for _, o := range objectives {
		if o.Status == objective.Active {
			r.Objectives = append(r.Objectives, ObjectiveRow{ID: o.ID, Name: o.Name, Progress: objective.Progress(o), OnTrack: objective.IsOnTrack(o, today)})
		}
	}
	return r, nil
}
