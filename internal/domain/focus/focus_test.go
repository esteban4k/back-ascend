package focus_test

import (
	"testing"

	"ascend/internal/domain/focus"
	"ascend/internal/domain/shared"
)

const minute = int64(60_000)

func TestXPCurve(t *testing.T) {
	cases := map[int]int{0: 0, 9*60 + 59: 0, 10 * 60: 17, 60 * 60: 100, 90 * 60: 150}
	for seconds, want := range cases {
		if got := focus.XP(seconds); got != want {
			t.Errorf("XP(%d) = %d, want %d", seconds, got, want)
		}
	}
}

func TestTimerPauseAndFinish(t *testing.T) {
	start := int64(1_700_000_000_000)
	timer, err := focus.StartTimer(focus.TimerInput{Label: " Build ", DomainID: "career", PlannedMinutes: 30}, start)
	if err != nil {
		t.Fatal(err)
	}
	timer = focus.Pause(timer, start+10*minute)
	timer = focus.Resume(timer, start+15*minute)
	if got := focus.ElapsedSeconds(timer, start+20*minute); got != 15*60 {
		t.Fatalf("elapsed = %d, want 900", got)
	}
	s := focus.Finish("s", timer, start+20*minute)
	if s.Status != focus.StatusStopped || s.Label != "Build" || s.XP != 25 {
		t.Fatalf("session = %+v", s)
	}
	full := focus.Finish("s", timer, start+60*minute)
	if full.Status != focus.StatusCompleted || full.FocusedSeconds != 30*60 {
		t.Fatalf("full session = %+v", full)
	}
}

func TestTimerValidation(t *testing.T) {
	if _, err := focus.StartTimer(focus.TimerInput{Label: "", PlannedMinutes: 25}, 1); !shared.IsValidation(err) {
		t.Fatalf("empty label err = %v", err)
	}
	if _, err := focus.StartTimer(focus.TimerInput{Label: "x", PlannedMinutes: 241}, 1); !shared.IsValidation(err) {
		t.Fatalf("too long err = %v", err)
	}
	future := focus.Timer{Label: "x", PlannedMinutes: 25, StartedAt: 2_000}
	if err := focus.ValidateTimer(future, 1_000); !shared.IsValidation(err) {
		t.Fatalf("future start err = %v", err)
	}
}
