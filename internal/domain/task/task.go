// Package task models one-off actions scheduled for a specific day.
package task

import (
	"strings"

	"ascend/internal/domain/shared"
)

// Task is a one-off action scheduled for a specific day.
type Task struct {
	ID       string           `json:"id"`
	Title    string           `json:"title"`
	DomainID string           `json:"domainId"`
	XP       int              `json:"xp"`
	Date     shared.LocalDate `json:"date"`
	// Time is `HH:mm`; empty means any time that day.
	Time            string `json:"time,omitempty"`
	DurationMinutes *int   `json:"durationMinutes,omitempty"`
	ObjectiveID     string `json:"objectiveId,omitempty"`
	Done            bool   `json:"done"`
}

// Draft is what the user provides to create a task.
type Draft struct {
	Title           string           `json:"title"`
	DomainID        string           `json:"domainId"`
	XP              int              `json:"xp"`
	Date            shared.LocalDate `json:"date"`
	Time            string           `json:"time,omitempty"`
	DurationMinutes *int             `json:"durationMinutes,omitempty"`
	ObjectiveID     string           `json:"objectiveId,omitempty"`
}

// Validate returns the first broken rule, or nil.
func Validate(d Draft) error {
	switch {
	case strings.TrimSpace(d.Title) == "":
		return shared.Invalid("Give the task a title")
	case strings.TrimSpace(d.DomainID) == "":
		return shared.Invalid("Pick a domain")
	case d.Date == "":
		return shared.Invalid("Pick a date")
	case !d.Date.IsValid():
		return shared.Invalid("The date is not valid")
	case d.XP < 0 || d.XP > 500:
		return shared.Invalid("XP must be between 0 and 500")
	case d.Time != "" && !shared.IsClockTime(d.Time):
		return shared.Invalid("Time must be HH:mm")
	case d.DurationMinutes != nil && *d.DurationMinutes <= 0:
		return shared.Invalid("Duration must be positive")
	}
	return nil
}

// New creates a task from a draft.
func New(id string, d Draft) (Task, error) {
	if err := Validate(d); err != nil {
		return Task{}, err
	}
	var duration *int
	if d.DurationMinutes != nil {
		v := *d.DurationMinutes
		duration = &v
	}
	return Task{
		ID:              id,
		Title:           strings.TrimSpace(d.Title),
		DomainID:        d.DomainID,
		XP:              d.XP,
		Date:            d.Date,
		Time:            d.Time,
		DurationMinutes: duration,
		ObjectiveID:     d.ObjectiveID,
	}, nil
}

// Complete marks the task done.
func Complete(t Task) (Task, error) {
	if t.Done {
		return Task{}, shared.Invalidf("%s is already complete", t.Title)
	}
	t.Done = true
	return t, nil
}

// Reopen marks the task open again.
func Reopen(t Task) (Task, error) {
	if !t.Done {
		return Task{}, shared.Invalidf("%s is still open", t.Title)
	}
	t.Done = false
	return t, nil
}
