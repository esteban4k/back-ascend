// Package activity models the immutable log of everything the system registered.
package activity

import "time"

// Kind classifies an activity event.
type Kind string

const (
	KindHabit     Kind = "habit"
	KindTask      Kind = "task"
	KindFocus     Kind = "focus"
	KindJournal   Kind = "journal"
	KindObjective Kind = "objective"
	KindWorkout   Kind = "workout"
	KindLevel     Kind = "level"
)

// Event is an immutable record of something the system registered.
type Event struct {
	ID    string `json:"id"`
	Kind  Kind   `json:"kind"`
	Title string `json:"title"`
	// Detail is an optional secondary line, e.g. "+2% · Reach 65kg".
	Detail   string `json:"detail,omitempty"`
	XP       int    `json:"xp"`
	DomainID string `json:"domainId,omitempty"`
	// RefID is the entity the event refers to, used to undo it.
	RefID string    `json:"refId,omitempty"`
	At    time.Time `json:"at"`
}
