package progression_test

import (
	"testing"

	"ascend/internal/domain/progression"
)

func TestLevel17NeedsTenThousandXP(t *testing.T) {
	if got := progression.XPToAdvance(17); got != 10_000 {
		t.Fatalf("XPToAdvance(17) = %d", got)
	}
}

func TestLevelFromTotalXP(t *testing.T) {
	s := progression.LevelFromTotalXP(progression.TotalXPForLevel(17) + 7_420)
	if s.Level != 17 || s.XPInLevel != 7_420 || s.XPForNext != 10_000 {
		t.Fatalf("state = %+v", s)
	}
}

func TestLevelUpFlag(t *testing.T) {
	_, award := progression.AwardXP(progression.TotalXPForLevel(3)-10, 40)
	if !award.LeveledUp || award.After.Level != 3 {
		t.Fatalf("award = %+v", award)
	}
}

func TestXPNeverGoesNegative(t *testing.T) {
	total, award := progression.AwardXP(30, -100)
	if total != 0 || award.After.Level != 1 {
		t.Fatalf("total = %d, award = %+v", total, award)
	}
}
