// Package journey models the moments worth remembering in the user's history.
package journey

import "ascend/internal/domain/shared"

// Kind classifies a milestone.
type Kind string

const (
	KindOrigin    Kind = "origin"
	KindLevel     Kind = "level"
	KindStreak    Kind = "streak"
	KindObjective Kind = "objective"
	KindFocus     Kind = "focus"
	KindRecord    Kind = "record"
)

// Milestone is a moment worth remembering.
type Milestone struct {
	ID          string           `json:"id"`
	Level       int              `json:"level"`
	Kind        Kind             `json:"kind"`
	Title       string           `json:"title"`
	Description string           `json:"description"`
	Date        shared.LocalDate `json:"date"`
}

// StreakMilestones are streak lengths that become milestones the first time they're reached.
var StreakMilestones = []int{7, 30, 100}

// StreakMilestoneReached returns the milestone crossed going from `previous` to `next`.
func StreakMilestoneReached(previous, next int) (int, bool) {
	for _, m := range StreakMilestones {
		if previous < m && next >= m {
			return m, true
		}
	}
	return 0, false
}

// FocusMilestoneHours are cumulative focus totals that become milestones.
var FocusMilestoneHours = []float64{10, 50, 100, 250}

// FocusMilestoneReached returns the focus milestone crossed going from `previous` to `next` hours.
func FocusMilestoneReached(previous, next float64) (float64, bool) {
	for _, h := range FocusMilestoneHours {
		if previous < h && next >= h {
			return h, true
		}
	}
	return 0, false
}
