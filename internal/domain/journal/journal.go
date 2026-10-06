// Package journal models daily reflection entries.
package journal

import (
	"strings"
	"time"

	"ascend/internal/domain/shared"
)

// Entry is one journal entry.
type Entry struct {
	ID    string           `json:"id"`
	Date  shared.LocalDate `json:"date"`
	Title string           `json:"title"`
	// Content is the free-form body.
	Content string `json:"content"`
	// Mood is 1 (low) – 5 (high).
	Mood int `json:"mood"`
	// Energy is 1 (low) – 5 (high).
	Energy     int       `json:"energy"`
	Wins       []string  `json:"wins"`
	Problems   []string  `json:"problems"`
	Reflection string    `json:"reflection"`
	Tomorrow   []string  `json:"tomorrow"`
	CreatedAt  time.Time `json:"createdAt"`
}

// Draft is the editable part of an entry.
type Draft struct {
	Date       shared.LocalDate `json:"date"`
	Title      string           `json:"title"`
	Content    string           `json:"content"`
	Mood       int              `json:"mood"`
	Energy     int              `json:"energy"`
	Wins       []string         `json:"wins"`
	Problems   []string         `json:"problems"`
	Reflection string           `json:"reflection"`
	Tomorrow   []string         `json:"tomorrow"`
}

// EntryXP is the XP associated with journaling (granted through the Journal habit).
const EntryXP = 30

// MoodLabels names each mood level.
var MoodLabels = map[int]string{1: "Drained", 2: "Low", 3: "Steady", 4: "Good", 5: "Sharp"}

// EnergyLabels names each energy level.
var EnergyLabels = map[int]string{1: "Empty", 2: "Low", 3: "Moderate", 4: "High", 5: "Peak"}

const untitled = "Untitled entry"

func anyFilled(items []string) bool {
	for _, i := range items {
		if strings.TrimSpace(i) != "" {
			return true
		}
	}
	return false
}

// Validate returns the first broken rule, or nil.
func Validate(d Draft) error {
	hasBody := strings.TrimSpace(d.Content) != "" ||
		strings.TrimSpace(d.Reflection) != "" ||
		anyFilled(d.Wins) ||
		anyFilled(d.Problems)
	switch {
	case !d.Date.IsValid():
		return shared.Invalid("Pick a valid date")
	case d.Mood < 1 || d.Mood > 5:
		return shared.Invalid("Mood must be between 1 and 5")
	case d.Energy < 1 || d.Energy > 5:
		return shared.Invalid("Energy must be between 1 and 5")
	case !hasBody:
		return shared.Invalid("Write something before saving")
	}
	return nil
}

func clean(items []string) []string {
	out := []string{}
	for _, i := range items {
		if t := strings.TrimSpace(i); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func title(t string) string {
	if t = strings.TrimSpace(t); t != "" {
		return t
	}
	return untitled
}

// New creates an entry from a draft.
func New(id string, d Draft, now time.Time) (Entry, error) {
	if err := Validate(d); err != nil {
		return Entry{}, err
	}
	return Entry{
		ID:         id,
		Date:       d.Date,
		Title:      title(d.Title),
		Content:    strings.TrimSpace(d.Content),
		Mood:       d.Mood,
		Energy:     d.Energy,
		Wins:       clean(d.Wins),
		Problems:   clean(d.Problems),
		Reflection: strings.TrimSpace(d.Reflection),
		Tomorrow:   clean(d.Tomorrow),
		CreatedAt:  now,
	}, nil
}

// Update applies a draft to an existing entry.
func Update(e Entry, d Draft) (Entry, error) {
	if err := Validate(d); err != nil {
		return Entry{}, err
	}
	e.Date = d.Date
	e.Title = title(d.Title)
	e.Content = strings.TrimSpace(d.Content)
	e.Mood = d.Mood
	e.Energy = d.Energy
	e.Wins = clean(d.Wins)
	e.Problems = clean(d.Problems)
	e.Reflection = strings.TrimSpace(d.Reflection)
	e.Tomorrow = clean(d.Tomorrow)
	return e, nil
}
