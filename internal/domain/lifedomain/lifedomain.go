// Package lifedomain models the life areas the user invests energy into
// (Physical, Career, …). Named this way to avoid confusion with the
// architectural "domain" layer.
package lifedomain

import (
	"math"
	"slices"
	"strings"

	"ascend/internal/domain/shared"
)

// LifeDomain is a life area.
type LifeDomain struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// Score is 0–100.
	Score float64 `json:"score"`
	// History holds weekly score snapshots, oldest first. The last entry is last week.
	History []float64 `json:"history"`
}

// Draft is the editable part of a life domain.
type Draft struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// XPPerPoint is how much invested XP moves the score by one point.
const XPPerPoint = 100

// InitialScore is where new domains start.
const InitialScore = 50

// Invest moves the score by the XP invested (negative XP takes it back).
func Invest(d LifeDomain, xp int) LifeDomain {
	d.Score = shared.Clamp(shared.Round(d.Score+float64(xp)/XPPerPoint, 1), 0, 100)
	return d
}

// Direction is where a score is heading.
type Direction string

const (
	Up   Direction = "up"
	Down Direction = "down"
	Flat Direction = "flat"
)

// Trend compares the score with last week's snapshot.
type Trend struct {
	Direction Direction `json:"direction"`
	Delta     float64   `json:"delta"`
}

// TrendOf compares the current score with the latest weekly snapshot.
func TrendOf(d LifeDomain) Trend {
	previous := d.Score
	if len(d.History) > 0 {
		previous = d.History[len(d.History)-1]
	}
	delta := shared.Round(d.Score-previous, 1)
	direction := Flat
	switch {
	case math.Abs(delta) < 0.5:
	case delta > 0:
		direction = Up
	default:
		direction = Down
	}
	return Trend{Direction: direction, Delta: delta}
}

// Validate returns the first broken rule, or nil.
func Validate(d Draft) error {
	name := strings.TrimSpace(d.Name)
	if name == "" {
		return shared.Invalid("Name is required")
	}
	if len([]rune(name)) > 24 {
		return shared.Invalid("Keep the name under 24 characters")
	}
	return nil
}

// New creates a life domain from a draft.
func New(id string, d Draft, initialScore float64) (LifeDomain, error) {
	if err := Validate(d); err != nil {
		return LifeDomain{}, err
	}
	return LifeDomain{
		ID:          id,
		Name:        strings.TrimSpace(d.Name),
		Description: strings.TrimSpace(d.Description),
		Score:       initialScore,
		History:     []float64{initialScore},
	}, nil
}

// Rename applies a draft to an existing domain, keeping its score.
func Rename(d LifeDomain, draft Draft) (LifeDomain, error) {
	if err := Validate(draft); err != nil {
		return LifeDomain{}, err
	}
	d.Name = strings.TrimSpace(draft.Name)
	d.Description = strings.TrimSpace(draft.Description)
	d.History = slices.Clone(d.History)
	return d, nil
}
