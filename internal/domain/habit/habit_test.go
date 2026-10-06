package habit_test

import (
	"testing"

	"ascend/internal/domain/habit"
	"ascend/internal/domain/shared"
)

const today shared.LocalDate = "2026-09-24" // Thursday

func mustNew(t *testing.T, id string, d habit.Draft, created shared.LocalDate) habit.Habit {
	t.Helper()
	h, err := habit.New(id, d, created)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func mustComplete(t *testing.T, h habit.Habit, date shared.LocalDate) habit.Habit {
	t.Helper()
	h, err := habit.Complete(h, date)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestDailyStreakKeptWhileTodayIsOpen(t *testing.T) {
	h := mustNew(t, "h", habit.Draft{Name: "Read", DomainID: "intellect", XP: 40, Schedule: habit.Schedule{Kind: habit.Daily}}, today.AddDays(-10))
	for i := 1; i <= 5; i++ {
		h = mustComplete(t, h, today.AddDays(-i))
	}
	if got := habit.CurrentStreak(h, today); got != 5 {
		t.Fatalf("streak = %d, want 5", got)
	}
	if got := habit.CurrentStreak(mustComplete(t, h, today), today); got != 6 {
		t.Fatalf("streak after today = %d, want 6", got)
	}
}

func TestWeeklyHabitOnlyCountsScheduledDays(t *testing.T) {
	h := mustNew(t, "g", habit.Draft{Name: "Gym", DomainID: "physical", XP: 80, Schedule: habit.Schedule{Kind: habit.Weekly, Days: []int{0, 1, 3, 4}}}, today.AddDays(-30))
	if !habit.IsDueOn(h, today) {
		t.Fatal("Thursday should be due")
	}
	if habit.IsDueOn(h, today.AddDays(-1)) {
		t.Fatal("Wednesday should not be due")
	}
	h = mustComplete(t, h, today.AddDays(-2)) // Tuesday
	h = mustComplete(t, h, today.AddDays(-3)) // Monday
	if got := habit.CurrentStreak(h, today); got != 2 {
		t.Fatalf("streak = %d, want 2", got)
	}
	if got := habit.WeekProgress(h, today); got != (habit.WeekCount{Done: 2, Due: 4}) {
		t.Fatalf("week = %+v, want {2 4}", got)
	}
}

func TestRejectsEmptyName(t *testing.T) {
	_, err := habit.New("x", habit.Draft{Name: " ", DomainID: "d", XP: 10, Schedule: habit.Schedule{Kind: habit.Daily}}, today)
	if !shared.IsValidation(err) {
		t.Fatalf("err = %v, want validation error", err)
	}
}

func TestSevenWeekdaysBecomeDaily(t *testing.T) {
	h := mustNew(t, "x", habit.Draft{Name: "All", DomainID: "d", XP: 10, Schedule: habit.Schedule{Kind: habit.Weekly, Days: []int{6, 5, 4, 3, 2, 1, 0, 0}}}, today)
	if h.Schedule.Kind != habit.Daily || h.Schedule.Days != nil {
		t.Fatalf("schedule = %+v, want daily", h.Schedule)
	}
}

func TestCompleteTwiceFailsAndUncompleteRestores(t *testing.T) {
	h := mustNew(t, "x", habit.Draft{Name: "Read", DomainID: "d", XP: 10, Schedule: habit.Schedule{Kind: habit.Daily}}, today)
	done := mustComplete(t, h, today)
	if _, err := habit.Complete(done, today); !shared.IsValidation(err) {
		t.Fatalf("second completion err = %v", err)
	}
	undone, err := habit.Uncomplete(done, today)
	if err != nil || len(undone.Completions) != 0 {
		t.Fatalf("uncomplete = %+v, %v", undone.Completions, err)
	}
	if len(done.Completions) != 1 {
		t.Fatal("uncomplete mutated the original habit")
	}
}

func TestLongestStreakAndConsistency(t *testing.T) {
	h := mustNew(t, "x", habit.Draft{Name: "Read", DomainID: "d", XP: 10, Schedule: habit.Schedule{Kind: habit.Daily}}, today.AddDays(-10))
	for _, d := range []int{-10, -9, -8, -7, -5, -4} {
		h = mustComplete(t, h, today.AddDays(d))
	}
	if got := habit.LongestStreak(h, today); got != 4 {
		t.Fatalf("longest = %d, want 4", got)
	}
	// 6 completions over the 10 due days before today (the created day is day -10).
	if got := habit.Consistency(h, today, 10); got != 0.6 {
		t.Fatalf("consistency = %v, want 0.6", got)
	}
}
