// Package progression holds the XP and level rules.
//
// Every level requires a bit more XP than the previous one:
//
//	xpToAdvance(level) = 600 · level − 200
//
// so level 1 → 2 costs 400 XP and level 17 → 18 costs 10,000 XP.
package progression

import "time"

// LevelState describes where a total amount of XP sits on the level curve.
type LevelState struct {
	Level int `json:"level"`
	// XPInLevel is the XP accumulated inside the current level.
	XPInLevel int `json:"xpInLevel"`
	// XPForNext is the XP required to reach the next level.
	XPForNext int `json:"xpForNext"`
	// Progress through the current level, 0–1.
	Progress float64 `json:"progress"`
}

// XPToAdvance is the XP needed to go from `level` to `level + 1`.
func XPToAdvance(level int) int {
	return 600*level - 200
}

// LevelFromTotalXP derives the level and in-level XP from total XP.
func LevelFromTotalXP(totalXP int) LevelState {
	level := 1
	remaining := max(0, totalXP)
	for remaining >= XPToAdvance(level) {
		remaining -= XPToAdvance(level)
		level++
	}
	next := XPToAdvance(level)
	return LevelState{Level: level, XPInLevel: remaining, XPForNext: next, Progress: float64(remaining) / float64(next)}
}

// TotalXPForLevel is the total XP at which `level` starts.
func TotalXPForLevel(level int) int {
	total := 0
	for l := 1; l < level; l++ {
		total += XPToAdvance(l)
	}
	return total
}

// XPAward is the before/after picture of granting XP.
type XPAward struct {
	Before    LevelState `json:"before"`
	After     LevelState `json:"after"`
	LeveledUp bool       `json:"leveledUp"`
}

// AwardXP adds `amount` (may be negative) to `totalXP`, never going below zero.
func AwardXP(totalXP, amount int) (int, XPAward) {
	before := LevelFromTotalXP(totalXP)
	next := max(0, totalXP+amount)
	after := LevelFromTotalXP(next)
	return next, XPAward{Before: before, After: after, LeveledUp: after.Level > before.Level}
}

// User is the person progressing through the system.
type User struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	TotalXP  int       `json:"totalXp"`
	JoinedAt time.Time `json:"joinedAt"`
}

// Level returns the user's position on the level curve.
func (u User) Level() LevelState {
	return LevelFromTotalXP(u.TotalXP)
}

// GrantXP returns the user with `amount` XP added, plus the award details.
func GrantXP(u User, amount int) (User, XPAward) {
	total, award := AwardXP(u.TotalXP, amount)
	u.TotalXP = total
	return u, award
}
