package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"ascend/internal/application"
	"ascend/internal/application/ports"
	"ascend/internal/domain/shared"
	"ascend/internal/infrastructure/seed"
	"ascend/internal/infrastructure/sqlite"
	"ascend/internal/infrastructure/system"
)

var now = time.Date(2026, 9, 24, 8, 0, 0, 0, time.FixedZone("COT", -5*3600))

func open(t *testing.T, path string) *sqlite.Store {
	t.Helper()
	s, err := sqlite.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func loaded(t *testing.T) (*sqlite.Store, ports.Ports, seed.Dataset) {
	t.Helper()
	s := open(t, ":memory:")
	data := seed.Demo(now)
	if err := s.Load(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	return s, s.Ports(system.FixedClock{At: now}, system.UUIDGenerator{}), data
}

func TestLoadRoundTripsEveryCollection(t *testing.T) {
	_, p, data := loaded(t)
	ctx := context.Background()

	habits, _ := p.Habits.GetAll(ctx)
	if !reflect.DeepEqual(habits, data.Habits) {
		t.Fatal("habits differ after a round trip")
	}
	workouts, _ := p.Workouts.GetAll(ctx)
	if !reflect.DeepEqual(workouts, data.Workouts) {
		t.Fatal("workouts differ after a round trip")
	}
	objectives, _ := p.Objectives.GetAll(ctx)
	if !reflect.DeepEqual(objectives, data.Objectives) {
		t.Fatal("objectives differ after a round trip")
	}
	user, err := p.Users.GetCurrent(ctx)
	if err != nil || user.ID != data.User.ID || !user.JoinedAt.Equal(data.User.JoinedAt) {
		t.Fatalf("user = %+v, %v", user, err)
	}
	events, _ := p.Activity.List(ctx, ports.ActivityQuery{})
	if len(events) != len(data.Activity) {
		t.Fatalf("activity = %d, want %d", len(events), len(data.Activity))
	}
	for i := 1; i < len(events); i++ {
		if events[i].At.After(events[i-1].At) {
			t.Fatal("activity is not newest first")
		}
	}
}

func TestQueries(t *testing.T) {
	_, p, _ := loaded(t)
	ctx := context.Background()

	tasks, _ := p.Tasks.GetBetween(ctx, "2026-09-24", "2026-09-24")
	if len(tasks) != 2 {
		t.Fatalf("today's tasks = %d, want 2", len(tasks))
	}
	since := now.AddDate(0, 0, -7)
	recent, _ := p.Activity.List(ctx, ports.ActivityQuery{Since: since, Limit: 5})
	if len(recent) != 5 || recent[4].At.Before(since) {
		t.Fatalf("recent = %d", len(recent))
	}
	found, _ := p.Activity.FindByRef(ctx, "habit:code:2026-09-23")
	if found == nil || found.XP != 100 {
		t.Fatalf("found = %+v", found)
	}
	missing, err := p.Habits.GetByID(ctx, "nope")
	if missing != nil || err != nil {
		t.Fatalf("missing = %+v, %v", missing, err)
	}
}

func TestTransactionsCommitAndRollBack(t *testing.T) {
	_, p, _ := loaded(t)
	ctx := context.Background()
	before, _ := p.Users.GetCurrent(ctx)

	boom := errors.New("boom")
	err := p.Tx.WithinTx(ctx, func(ctx context.Context) error {
		u := before
		u.TotalXP += 1000
		if err := p.Users.Save(ctx, u); err != nil {
			return err
		}
		// Nested units of work join the outer transaction.
		return p.Tx.WithinTx(ctx, func(ctx context.Context) error {
			if err := p.Habits.Delete(ctx, "gym"); err != nil {
				return err
			}
			return boom
		})
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	after, _ := p.Users.GetCurrent(ctx)
	gym, _ := p.Habits.GetByID(ctx, "gym")
	if after.TotalXP != before.TotalXP || gym == nil {
		t.Fatal("rolled-back writes are visible")
	}
}

func TestUseCasesRunOnSQLite(t *testing.T) {
	_, p, _ := loaded(t)
	ctx := context.Background()
	uc := application.New(p)
	before, _ := uc.Profile.Get(ctx)
	if _, err := uc.Habits.Complete(ctx, "read"); err != nil {
		t.Fatal(err)
	}
	if _, err := uc.Habits.Complete(ctx, "read"); !shared.IsValidation(err) {
		t.Fatalf("second completion err = %v", err)
	}
	after, _ := uc.Profile.Get(ctx)
	if after.User.TotalXP != before.User.TotalXP+40 {
		t.Fatalf("XP %d → %d", before.User.TotalXP, after.User.TotalXP)
	}
	if _, err := uc.Habits.Uncomplete(ctx, "read"); err != nil {
		t.Fatal(err)
	}
	reverted, _ := uc.Profile.Get(ctx)
	if reverted.User.TotalXP != before.User.TotalXP {
		t.Fatalf("XP after undo = %d", reverted.User.TotalXP)
	}
	if _, err := uc.Performance.Report(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestDataSurvivesReopening(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "ascend.db")
	ctx := context.Background()

	first, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if empty, _ := first.Empty(ctx); !empty {
		t.Fatal("a new database should be empty")
	}
	if err := first.Load(ctx, seed.Blank(now, "Ana")); err != nil {
		t.Fatal(err)
	}
	first.Close()

	second := open(t, path) // migrations must be idempotent
	if empty, _ := second.Empty(ctx); empty {
		t.Fatal("data was lost")
	}
	user, err := second.Ports(nil, nil).Users.GetCurrent(ctx)
	if err != nil || user.Name != "Ana" {
		t.Fatalf("user = %+v, %v", user, err)
	}
}
