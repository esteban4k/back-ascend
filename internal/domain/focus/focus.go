// Package focus models deep-work sessions and the timer that produces them.
package focus

import (
	"strings"
	"time"

	"ascend/internal/domain/shared"
)

// Status is how a session ended.
type Status string

const (
	StatusCompleted Status = "completed"
	StatusStopped   Status = "stopped"
)

// Session is a finished block of focused work.
type Session struct {
	ID             string    `json:"id"`
	Label          string    `json:"label"`
	DomainID       string    `json:"domainId"`
	PlannedMinutes int       `json:"plannedMinutes"`
	StartedAt      time.Time `json:"startedAt"`
	EndedAt        time.Time `json:"endedAt"`
	FocusedSeconds int       `json:"focusedSeconds"`
	Status         Status    `json:"status"`
	XP             int       `json:"xp"`
}

// MinRewardedMinutes is the shortest session that earns XP.
const MinRewardedMinutes = 10

// Presets are the durations offered by default, in minutes.
var Presets = []int{25, 50, 90}

// XP converts focused time into XP: 60 focused minutes = 100 XP.
func XP(focusedSeconds int) int {
	minutes := focusedSeconds / 60
	if minutes < MinRewardedMinutes {
		return 0
	}
	return shared.RoundInt(float64(minutes*5) / 3)
}

// ─── Timer ────────────────────────────────────────────────────────────────
// A running session is a value object; elapsed time is derived from
// timestamps rather than ticks, so it survives reloads and navigation.
// Timestamps are epoch milliseconds, the same representation the web client
// keeps while the timer runs.

// Timer is a running (or paused) focus session.
type Timer struct {
	Label          string `json:"label"`
	DomainID       string `json:"domainId"`
	PlannedMinutes int    `json:"plannedMinutes"`
	// StartedAt is epoch ms.
	StartedAt int64 `json:"startedAt"`
	// PausedAt is epoch ms when paused, nil otherwise.
	PausedAt *int64 `json:"pausedAt"`
	// PausedMs is the total time spent paused so far.
	PausedMs int64 `json:"pausedMs"`
}

// TimerInput is what the user chooses before starting.
type TimerInput struct {
	Label          string `json:"label"`
	DomainID       string `json:"domainId"`
	PlannedMinutes int    `json:"plannedMinutes"`
}

func validateInput(label string, plannedMinutes int) error {
	if strings.TrimSpace(label) == "" {
		return shared.Invalid("Name what you are focusing on")
	}
	if plannedMinutes < 1 || plannedMinutes > 240 {
		return shared.Invalid("Sessions must be between 1 and 240 minutes")
	}
	return nil
}

// StartTimer starts a timer at `now` (epoch ms).
func StartTimer(input TimerInput, now int64) (Timer, error) {
	if err := validateInput(input.Label, input.PlannedMinutes); err != nil {
		return Timer{}, err
	}
	return Timer{
		Label:          strings.TrimSpace(input.Label),
		DomainID:       input.DomainID,
		PlannedMinutes: input.PlannedMinutes,
		StartedAt:      now,
	}, nil
}

// ValidateTimer checks a timer handed back by a client before finishing it.
func ValidateTimer(t Timer, now int64) error {
	if err := validateInput(t.Label, t.PlannedMinutes); err != nil {
		return err
	}
	if t.StartedAt <= 0 || t.StartedAt > now {
		return shared.Invalid("The session start time is not valid")
	}
	if t.PausedMs < 0 || (t.PausedAt != nil && (*t.PausedAt < t.StartedAt || *t.PausedAt > now)) {
		return shared.Invalid("The session pause time is not valid")
	}
	return nil
}

// Pause freezes the timer at `now`.
func Pause(t Timer, now int64) Timer {
	if t.PausedAt != nil {
		return t
	}
	t.PausedAt = &now
	return t
}

// Resume continues a paused timer at `now`.
func Resume(t Timer, now int64) Timer {
	if t.PausedAt == nil {
		return t
	}
	t.PausedMs += now - *t.PausedAt
	t.PausedAt = nil
	return t
}

// ElapsedSeconds is the focused time so far, capped at the planned duration.
func ElapsedSeconds(t Timer, now int64) int {
	end := now
	if t.PausedAt != nil {
		end = *t.PausedAt
	}
	ms := end - t.StartedAt - t.PausedMs
	return min(t.PlannedMinutes*60, max(0, int(ms/1000)))
}

// RemainingSeconds is the planned time left.
func RemainingSeconds(t Timer, now int64) int {
	return t.PlannedMinutes*60 - ElapsedSeconds(t, now)
}

// IsFinished reports whether the planned time is used up.
func IsFinished(t Timer, now int64) bool {
	return RemainingSeconds(t, now) <= 0
}

// Finish turns the timer into a recorded session.
func Finish(id string, t Timer, now int64) Session {
	focused := ElapsedSeconds(t, now)
	status := StatusStopped
	if focused >= t.PlannedMinutes*60 {
		status = StatusCompleted
	}
	return Session{
		ID:             id,
		Label:          t.Label,
		DomainID:       t.DomainID,
		PlannedMinutes: t.PlannedMinutes,
		StartedAt:      time.UnixMilli(t.StartedAt).UTC(),
		EndedAt:        time.UnixMilli(now).UTC(),
		FocusedSeconds: focused,
		Status:         status,
		XP:             XP(focused),
	}
}
