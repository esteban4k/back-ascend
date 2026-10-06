// Package objective models measurable targets with a deadline.
package objective

import (
	"math"
	"slices"
	"strings"

	"ascend/internal/domain/shared"
)

// Status is where an objective stands.
type Status string

const (
	Active    Status = "active"
	Completed Status = "completed"
	Paused    Status = "paused"
)

// IsValid reports whether s is a known status.
func (s Status) IsValid() bool {
	return s == Active || s == Completed || s == Paused
}

// Metric is a measurable target, e.g. body weight going from 56.6 kg to 65 kg.
type Metric struct {
	Unit    string  `json:"unit"`
	Start   float64 `json:"start"`
	Current float64 `json:"current"`
	Target  float64 `json:"target"`
}

// Milestone is a checkpoint on the way to an objective.
type Milestone struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

// Objective is a measurable target with a deadline.
type Objective struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	DomainID    string           `json:"domainId"`
	Deadline    shared.LocalDate `json:"deadline"`
	Status      Status           `json:"status"`
	// Metric, when present, measures progress; otherwise milestones do.
	Metric      *Metric          `json:"metric,omitempty"`
	Milestones  []Milestone      `json:"milestones"`
	HabitIDs    []string         `json:"habitIds"`
	CreatedAt   shared.LocalDate `json:"createdAt"`
	CompletedAt shared.LocalDate `json:"completedAt,omitempty"`
}

// Draft is what the user provides to create an objective.
type Draft struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	DomainID    string           `json:"domainId"`
	Deadline    shared.LocalDate `json:"deadline"`
	Metric      *Metric          `json:"metric,omitempty"`
	Milestones  []string         `json:"milestones"`
	HabitIDs    []string         `json:"habitIds"`
}

// CompletionXP is granted when an objective is completed.
const CompletionXP = 500

func nonEmpty(items []string) []string {
	out := []string{}
	for _, i := range items {
		if t := strings.TrimSpace(i); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// Validate returns the first broken rule, or nil.
func Validate(d Draft) error {
	switch {
	case strings.TrimSpace(d.Name) == "":
		return shared.Invalid("Give the objective a name")
	case strings.TrimSpace(d.DomainID) == "":
		return shared.Invalid("Pick a domain")
	case d.Deadline == "":
		return shared.Invalid("Set a deadline")
	case !d.Deadline.IsValid():
		return shared.Invalid("The deadline is not a valid date")
	}
	if d.Metric != nil {
		m := d.Metric
		if strings.TrimSpace(m.Unit) == "" {
			return shared.Invalid("Set a unit for the metric")
		}
		if !finite(m.Start) || !finite(m.Current) || !finite(m.Target) {
			return shared.Invalid("Enter valid numbers for the metric")
		}
		if m.Start == m.Target {
			return shared.Invalid("Start and target must be different")
		}
	}
	if d.Metric == nil && len(nonEmpty(d.Milestones)) == 0 {
		return shared.Invalid("Add a metric or at least one milestone")
	}
	return nil
}

// New creates an objective from a draft.
func New(id string, d Draft, today shared.LocalDate, newID func() string) (Objective, error) {
	if err := Validate(d); err != nil {
		return Objective{}, err
	}
	var metric *Metric
	if d.Metric != nil {
		m := *d.Metric
		m.Unit = strings.TrimSpace(m.Unit)
		metric = &m
	}
	milestones := []Milestone{}
	for _, title := range nonEmpty(d.Milestones) {
		milestones = append(milestones, Milestone{ID: newID(), Title: title})
	}
	habitIDs := slices.Compact(slices.Clone(d.HabitIDs))
	if habitIDs == nil {
		habitIDs = []string{}
	}
	o := Objective{
		ID:          id,
		Name:        strings.TrimSpace(d.Name),
		Description: strings.TrimSpace(d.Description),
		DomainID:    d.DomainID,
		Deadline:    d.Deadline,
		Status:      Active,
		Metric:      metric,
		Milestones:  milestones,
		HabitIDs:    habitIDs,
		CreatedAt:   today,
	}
	return o, nil
}

// Progress is 0–1.
func Progress(o Objective) float64 {
	if o.Status == Completed {
		return 1
	}
	if o.Metric != nil {
		m := o.Metric
		return shared.Clamp((m.Current-m.Start)/(m.Target-m.Start), 0, 1)
	}
	if len(o.Milestones) == 0 {
		return 0
	}
	done := 0
	for _, m := range o.Milestones {
		if m.Done {
			done++
		}
	}
	return float64(done) / float64(len(o.Milestones))
}

// DaysRemaining counts days until the deadline (negative once it has passed).
func DaysRemaining(o Objective, today shared.LocalDate) int {
	return shared.DaysBetween(today, o.Deadline)
}

// IsOnTrack reports whether progress keeps pace with the time elapsed since
// creation. Used to flag objectives that need attention.
func IsOnTrack(o Objective, today shared.LocalDate) bool {
	total := shared.DaysBetween(o.CreatedAt, o.Deadline)
	if total <= 0 {
		return Progress(o) >= 1
	}
	elapsed := shared.Clamp(float64(shared.DaysBetween(o.CreatedAt, today))/float64(total), 0, 1)
	return Progress(o)+0.05 >= elapsed
}

func clone(o Objective) Objective {
	if o.Metric != nil {
		m := *o.Metric
		o.Metric = &m
	}
	o.Milestones = slices.Clone(o.Milestones)
	o.HabitIDs = slices.Clone(o.HabitIDs)
	return o
}

// settle completes or reopens the objective depending on its progress.
func settle(o Objective, today shared.LocalDate) Objective {
	probe := o
	probe.Status = Active
	reached := Progress(probe) >= 1
	switch {
	case reached && o.Status != Completed:
		o.Status = Completed
		o.CompletedAt = today
	case !reached && o.Status == Completed:
		o.Status = Active
		o.CompletedAt = ""
	}
	return o
}

// RecordMetricValue records a new measurement.
func RecordMetricValue(o Objective, value float64, today shared.LocalDate) (Objective, error) {
	if o.Metric == nil {
		return Objective{}, shared.Invalid("This objective is measured by milestones")
	}
	if !finite(value) {
		return Objective{}, shared.Invalid("Enter a valid number")
	}
	o = clone(o)
	o.Metric.Current = shared.Round(value, 2)
	return settle(o, today), nil
}

// ToggleMilestone flips a milestone. Milestones only drive completion when
// there is no metric.
func ToggleMilestone(o Objective, milestoneID string, today shared.LocalDate) (Objective, error) {
	o = clone(o)
	i := slices.IndexFunc(o.Milestones, func(m Milestone) bool { return m.ID == milestoneID })
	if i < 0 {
		return Objective{}, shared.NotFound("Milestone", milestoneID)
	}
	o.Milestones[i].Done = !o.Milestones[i].Done
	if o.Metric != nil {
		return o, nil
	}
	return settle(o, today), nil
}

// SetStatus changes the status directly (pause, resume, mark complete).
func SetStatus(o Objective, status Status, today shared.LocalDate) (Objective, error) {
	if !status.IsValid() {
		return Objective{}, shared.Invalid("Unknown status")
	}
	o = clone(o)
	o.Status = status
	switch {
	case status == Completed && o.CompletedAt == "":
		o.CompletedAt = today
	case status != Completed:
		o.CompletedAt = ""
	}
	return o, nil
}

// WithoutHabit removes a habit link.
func WithoutHabit(o Objective, habitID string) Objective {
	o = clone(o)
	o.HabitIDs = slices.DeleteFunc(o.HabitIDs, func(id string) bool { return id == habitID })
	return o
}
