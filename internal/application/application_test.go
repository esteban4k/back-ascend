package application_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"ascend/internal/application"
	"ascend/internal/application/ports"
	"ascend/internal/application/today"
	"ascend/internal/domain/focus"
	"ascend/internal/domain/habit"
	"ascend/internal/domain/journal"
	"ascend/internal/domain/journey"
	"ascend/internal/domain/lifedomain"
	"ascend/internal/domain/objective"
	"ascend/internal/domain/shared"
	"ascend/internal/domain/task"
	"ascend/internal/domain/training"
	"ascend/internal/infrastructure/container"
	"ascend/internal/infrastructure/seed"
)

// Use cases run against the in-memory adapter and a frozen clock. No HTTP involved.

var bogota = time.FixedZone("COT", -5*3600)

func setup(t *testing.T) (*application.UseCases, ports.Ports, context.Context) {
	t.Helper()
	now := time.Date(2026, 9, 24, 8, 0, 0, 0, bogota)
	uc, p := container.ForTest(seed.Demo(now), now)
	return uc, p, context.Background()
}

type journeyMilestone = journey.Milestone

func lifeDomainDraft(name string) lifedomain.Draft { return lifedomain.Draft{Name: name} }

// must unwraps a use case result; an unexpected error fails the test with a panic.
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func TestCompletingAHabitGrantsXPLogsActivityAndExtendsTheStreak(t *testing.T) {
	uc, _, ctx := setup(t)
	before := must(uc.Profile.Get(ctx))
	read := must(uc.Habits.Get(ctx, "read"))

	reward := must(uc.Habits.Complete(ctx, "read"))

	after := must(uc.Profile.Get(ctx))
	if reward.XP != 40 || reward.Streak == nil || *reward.Streak != read.Streak+1 {
		t.Fatalf("reward = %+v, streak before = %d", reward, read.Streak)
	}
	if after.User.TotalXP-before.User.TotalXP != 40 {
		t.Fatalf("XP gained = %d", after.User.TotalXP-before.User.TotalXP)
	}
	latest := must(uc.Profile.Activity(ctx, 1))
	if len(latest) != 1 || latest[0].Title != "Completed Read 20 min" {
		t.Fatalf("latest activity = %+v", latest)
	}
}

func TestUndoingACompletionRevokesExactlyWhatWasGranted(t *testing.T) {
	uc, p, ctx := setup(t)
	before := must(uc.Profile.Get(ctx))
	domainBefore := must(p.Domains.GetByID(ctx, "intellect"))
	must(uc.Habits.Complete(ctx, "read"))
	must(uc.Habits.Uncomplete(ctx, "read"))
	after := must(uc.Profile.Get(ctx))
	domainAfter := must(p.Domains.GetByID(ctx, "intellect"))
	if after.User.TotalXP != before.User.TotalXP {
		t.Fatalf("XP %d → %d", before.User.TotalXP, after.User.TotalXP)
	}
	if domainAfter.Score != domainBefore.Score {
		t.Fatalf("domain score %v → %v", domainBefore.Score, domainAfter.Score)
	}
}

func TestTodayProgressReflectsCompletedMissions(t *testing.T) {
	uc, _, ctx := setup(t)
	before := must(uc.Today.Get(ctx))
	i := slices.IndexFunc(before.Missions, func(m today.Mission) bool { return !m.Done && m.Source == today.SourceHabit })
	if i < 0 {
		t.Fatal("no open habit mission")
	}
	open := before.Missions[i]
	must(uc.Habits.Complete(ctx, open.ID))
	after := must(uc.Today.Get(ctx))
	if after.Completed != before.Completed+1 || after.XPEarned != before.XPEarned+open.XP {
		t.Fatalf("before %+v after %+v", before, after)
	}
}

func TestSavingAJournalEntryCompletesTheJournalHabit(t *testing.T) {
	uc, _, ctx := setup(t)
	created := must(uc.Journal.Create(ctx, journal.Draft{
		Date: "2026-09-24", Title: "Test", Content: "Shipped the auth refactor.", Mood: 4, Energy: 3,
	}))
	if created.Reward == nil || created.Reward.HabitName != "Journal" {
		t.Fatalf("reward = %+v", created.Reward)
	}
	if created.Entry.Wins == nil || len(created.Entry.Wins) != 0 {
		t.Fatalf("wins should be an empty list, got %#v", created.Entry.Wins)
	}
	// A second entry the same day does not complete it twice.
	again := must(uc.Journal.Create(ctx, journal.Draft{Date: "2026-09-24", Content: "More.", Mood: 3, Energy: 3}))
	if again.Reward != nil {
		t.Fatalf("second reward = %+v", again.Reward)
	}
}

func TestReachingAnObjectiveTargetCompletesItAndRecordsAMilestone(t *testing.T) {
	uc, _, ctx := setup(t)
	reward := must(uc.Objectives.RecordValue(ctx, "reach-65kg", 65.1))
	if reward == nil || reward.XP != 500 {
		t.Fatalf("reward = %+v", reward)
	}
	j := must(uc.Journey.Get(ctx))
	if !slices.ContainsFunc(j.Milestones, func(m journeyMilestone) bool { return m.ID == "objective-reach-65kg" }) {
		t.Fatal("journey milestone missing")
	}
}

func TestObjectiveProgressIsLoggedWithoutReward(t *testing.T) {
	uc, _, ctx := setup(t)
	reward := must(uc.Objectives.RecordValue(ctx, "reach-65kg", 61))
	if reward != nil {
		t.Fatalf("reward = %+v", reward)
	}
	latest := must(uc.Profile.Activity(ctx, 1))
	if latest[0].Title != "Objective progressed" || latest[0].Detail != "+10% · Reach 65 kg" {
		t.Fatalf("activity = %+v", latest[0])
	}
}

func TestFocusSessionsGrantXPProportionalToFocusedTime(t *testing.T) {
	uc, p, ctx := setup(t)
	timer := must(uc.Focus.Start(ctx, focus.TimerInput{Label: "Build auth", DomainID: "career", PlannedMinutes: 60}))
	timer.StartedAt = p.Clock.Now().UnixMilli() - 60*60*1000
	done := must(uc.Focus.Complete(ctx, timer))
	if done.Session.Status != focus.StatusCompleted || done.Reward == nil || done.Reward.XP != 100 {
		t.Fatalf("done = %+v reward = %+v", done.Session, done.Reward)
	}
}

func TestTasksCompleteReopenAndDelete(t *testing.T) {
	uc, _, ctx := setup(t)
	before := must(uc.Profile.Get(ctx))
	created := must(uc.Today.CreateTask(ctx, task.Draft{Title: "Write docs", DomainID: "career", XP: 50, Date: "2026-09-24", Time: "11:00"}))
	must(uc.Today.CompleteTask(ctx, created.ID))
	if _, err := uc.Today.CompleteTask(ctx, created.ID); !shared.IsValidation(err) {
		t.Fatalf("second completion err = %v", err)
	}
	if err := uc.Today.DeleteTask(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	after := must(uc.Profile.Get(ctx))
	if after.User.TotalXP != before.User.TotalXP {
		t.Fatalf("deleting a done task should revoke its XP: %d → %d", before.User.TotalXP, after.User.TotalXP)
	}
	if _, err := uc.Today.CompleteTask(ctx, created.ID); !shared.IsNotFound(err) {
		t.Fatalf("completing a deleted task err = %v", err)
	}
}

func TestDraftsMustReferenceExistingEntities(t *testing.T) {
	uc, _, ctx := setup(t)
	_, err := uc.Habits.Create(ctx, habit.Draft{Name: "Stretch", DomainID: "nope", XP: 20, Schedule: habit.Schedule{Kind: habit.Daily}})
	if !shared.IsValidation(err) {
		t.Fatalf("unknown domain err = %v", err)
	}
	_, err = uc.Objectives.Create(ctx, objective.Draft{Name: "X", DomainID: "career", Deadline: "2026-12-31", Milestones: []string{"a"}, HabitIDs: []string{"ghost"}})
	if !shared.IsValidation(err) {
		t.Fatalf("unknown habit err = %v", err)
	}
}

func TestDeletingAHabitUnlinksItFromObjectives(t *testing.T) {
	uc, p, ctx := setup(t)
	if err := uc.Habits.Delete(ctx, "gym"); err != nil {
		t.Fatal(err)
	}
	o := must(p.Objectives.GetByID(ctx, "reach-65kg"))
	if slices.Contains(o.HabitIDs, "gym") {
		t.Fatalf("habitIds = %v", o.HabitIDs)
	}
}

func TestDomainsInUseCannotBeDeleted(t *testing.T) {
	uc, _, ctx := setup(t)
	if err := uc.Domains.Delete(ctx, "career"); !shared.IsValidation(err) {
		t.Fatalf("err = %v", err)
	}
	created := must(uc.Domains.Create(ctx, lifeDomainDraft("Spirituality")))
	if err := uc.Domains.Delete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
}

func TestWorkoutFlowCompletesTheGymHabit(t *testing.T) {
	uc, _, ctx := setup(t)
	overview := must(uc.Training.Overview(ctx))
	if overview.Current != nil || len(overview.History) == 0 || len(overview.Templates) == 0 {
		t.Fatalf("overview = current %v, %d history, templates %v", overview.Current, len(overview.History), overview.Templates)
	}
	w := must(uc.Training.Start(ctx, "Upper A"))
	if len(w.Exercises) == 0 {
		t.Fatal("template exercises were not copied")
	}
	if _, err := uc.Training.Start(ctx, "Lower A"); !shared.IsValidation(err) {
		t.Fatalf("second start err = %v", err)
	}
	weight := 200.0
	logged := must(uc.Training.LogSet(ctx, w.ID, w.Exercises[0].ID, training.SetInput{Weight: &weight, Reps: 5}))
	if !logged.IsRecord {
		t.Fatal("200 kg bench should be a record")
	}
	// The seed already completed today's Gym at 07:00, so the workout earns base XP.
	reward := must(uc.Training.Finish(ctx, w.ID, 60))
	if reward.HabitName != "" || reward.XP != training.WorkoutXP {
		t.Fatalf("reward = %+v", reward)
	}
	if err := uc.Training.Discard(ctx, w.ID); !shared.IsValidation(err) {
		t.Fatalf("discarding a finished workout err = %v", err)
	}

	// With Gym still open, finishing a workout completes it instead.
	must(uc.Habits.Uncomplete(ctx, "gym"))
	next := must(uc.Training.Start(ctx, "Lower A"))
	must(uc.Training.LogSet(ctx, next.ID, next.Exercises[0].ID, training.SetInput{Weight: &weight, Reps: 3}))
	habitReward := must(uc.Training.Finish(ctx, next.ID, 50))
	if habitReward.HabitName != "Gym" || habitReward.Streak == nil {
		t.Fatalf("habit reward = %+v", habitReward)
	}
}

func TestFailedUnitOfWorkRollsBack(t *testing.T) {
	_, p, ctx := setup(t)
	before := must(p.Users.GetCurrent(ctx))
	err := p.Tx.WithinTx(ctx, func(ctx context.Context) error {
		u := before
		u.TotalXP += 1000
		if err := p.Users.Save(ctx, u); err != nil {
			return err
		}
		return shared.Invalid("boom")
	})
	if !shared.IsValidation(err) {
		t.Fatalf("err = %v", err)
	}
	after := must(p.Users.GetCurrent(ctx))
	if after.TotalXP != before.TotalXP {
		t.Fatalf("XP %d → %d after rollback", before.TotalXP, after.TotalXP)
	}
}

func TestReadModels(t *testing.T) {
	uc, _, ctx := setup(t)
	agenda := must(uc.Calendar.Agenda(ctx, "2026-09-21", "2026-09-27"))
	if len(agenda) == 0 {
		t.Fatal("empty agenda")
	}
	if _, err := uc.Calendar.Agenda(ctx, "2026-09-27", "2026-09-21"); !shared.IsValidation(err) {
		t.Fatalf("reversed range err = %v", err)
	}
	report := must(uc.Performance.Report(ctx))
	if len(report.DailyXP) != 30 || len(report.WeekComparison) != 7 || len(report.MonthlyXP) != 6 || len(report.WeeklyConsistency) != 12 {
		t.Fatalf("report shape: %d daily, %d week, %d monthly, %d weekly", len(report.DailyXP), len(report.WeekComparison), len(report.MonthlyXP), len(report.WeeklyConsistency))
	}
	if report.WeekComparison[6].ThisWeek != nil {
		t.Fatal("Sunday is still ahead and should be null")
	}
	cells := must(uc.Habits.Consistency(ctx, 140, ""))
	if len(cells) != 140 || cells[139].Date != "2026-09-24" {
		t.Fatalf("cells = %d, last %s", len(cells), cells[len(cells)-1].Date)
	}
	domains := must(uc.Domains.List(ctx))
	if len(domains) != 6 {
		t.Fatalf("domains = %d", len(domains))
	}
	current := must(uc.Objectives.Current(ctx))
	if current == nil || current.Objective.ID != "reach-65kg" {
		t.Fatalf("current objective = %+v", current)
	}
	fo := must(uc.Focus.Overview(ctx))
	if fo.TotalHours < 100 || len(fo.History) != 30 {
		t.Fatalf("focus overview hours %v history %d", fo.TotalHours, len(fo.History))
	}
	j := must(uc.Journey.Get(ctx))
	if j.Stats.DaysActive != 197 || j.Level.Level != 17 {
		t.Fatalf("journey stats = %+v level = %d", j.Stats, j.Level.Level)
	}
}
