// Package training models workouts, sets and personal records.
package training

import (
	"slices"
	"strings"

	"ascend/internal/domain/shared"
)

// Set is one logged set of an exercise.
type Set struct {
	ID string `json:"id"`
	// Weight is in kg; nil means bodyweight.
	Weight *float64 `json:"weight"`
	Reps   int      `json:"reps"`
	// RIR is reps in reserve, nil when not tracked.
	RIR *int `json:"rir"`
}

// SetInput is a set before it gets an id.
type SetInput struct {
	Weight *float64 `json:"weight"`
	Reps   int      `json:"reps"`
	RIR    *int     `json:"rir"`
}

// Exercise groups the sets of one movement.
type Exercise struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Sets []Set  `json:"sets"`
}

// Status is whether a workout is still running.
type Status string

const (
	InProgress Status = "in-progress"
	Completed  Status = "completed"
)

// Workout is one training session.
type Workout struct {
	ID              string           `json:"id"`
	Name            string           `json:"name"`
	Date            shared.LocalDate `json:"date"`
	Status          Status           `json:"status"`
	Exercises       []Exercise       `json:"exercises"`
	DurationMinutes *int             `json:"durationMinutes,omitempty"`
}

// WorkoutXP is granted for a workout when no habit is wired to training.
const WorkoutXP = 80

// DomainID is the life domain that training invests in.
const DomainID = "physical"

// ValidateSet returns the first broken rule, or nil.
func ValidateSet(s SetInput) error {
	switch {
	case s.Reps <= 0 || s.Reps > 100:
		return shared.Invalid("Reps must be 1–100")
	case s.Weight != nil && (*s.Weight < 0 || *s.Weight > 500):
		return shared.Invalid("Weight must be 0–500 kg")
	case s.RIR != nil && (*s.RIR < 0 || *s.RIR > 10):
		return shared.Invalid("RIR must be 0–10")
	}
	return nil
}

func clone(w Workout) Workout {
	exercises := make([]Exercise, len(w.Exercises))
	for i, e := range w.Exercises {
		e.Sets = slices.Clone(e.Sets)
		if e.Sets == nil {
			e.Sets = []Set{}
		}
		exercises[i] = e
	}
	w.Exercises = exercises
	return w
}

func exerciseIndex(w Workout, exerciseID string) (int, error) {
	i := slices.IndexFunc(w.Exercises, func(e Exercise) bool { return e.ID == exerciseID })
	if i < 0 {
		return 0, shared.NotFound("Exercise", exerciseID)
	}
	return i, nil
}

// LogSet appends a set to an exercise of a running workout.
func LogSet(w Workout, exerciseID string, set Set) (Workout, error) {
	if err := ValidateSet(SetInput{Weight: set.Weight, Reps: set.Reps, RIR: set.RIR}); err != nil {
		return Workout{}, err
	}
	if w.Status == Completed {
		return Workout{}, shared.Invalid("This workout is already finished")
	}
	i, err := exerciseIndex(w, exerciseID)
	if err != nil {
		return Workout{}, err
	}
	w = clone(w)
	w.Exercises[i].Sets = append(w.Exercises[i].Sets, set)
	return w, nil
}

// RemoveSet deletes a set from an exercise.
func RemoveSet(w Workout, exerciseID, setID string) (Workout, error) {
	i, err := exerciseIndex(w, exerciseID)
	if err != nil {
		return Workout{}, err
	}
	if !slices.ContainsFunc(w.Exercises[i].Sets, func(s Set) bool { return s.ID == setID }) {
		return Workout{}, shared.NotFound("Set", setID)
	}
	w = clone(w)
	w.Exercises[i].Sets = slices.DeleteFunc(w.Exercises[i].Sets, func(s Set) bool { return s.ID == setID })
	return w, nil
}

// AddExercise appends an exercise to a running workout.
func AddExercise(w Workout, e Exercise) (Workout, error) {
	name := strings.TrimSpace(e.Name)
	if name == "" {
		return Workout{}, shared.Invalid("Name the exercise")
	}
	if w.Status == Completed {
		return Workout{}, shared.Invalid("This workout is already finished")
	}
	w = clone(w)
	e.Name = name
	if e.Sets == nil {
		e.Sets = []Set{}
	}
	w.Exercises = append(w.Exercises, e)
	return w, nil
}

// Complete finishes the workout.
func Complete(w Workout, durationMinutes int) (Workout, error) {
	if w.Status == Completed {
		return Workout{}, shared.Invalid("This workout is already finished")
	}
	if durationMinutes <= 0 || durationMinutes > 600 {
		return Workout{}, shared.Invalid("Duration must be between 1 and 600 minutes")
	}
	if TotalSets(w) == 0 {
		return Workout{}, shared.Invalid("Log at least one set first")
	}
	w = clone(w)
	w.Status = Completed
	w.DurationMinutes = &durationMinutes
	return w, nil
}

// New starts an empty workout.
func New(id, name string, date shared.LocalDate) Workout {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Workout"
	}
	return Workout{ID: id, Name: name, Date: date, Status: InProgress, Exercises: []Exercise{}}
}

// FromTemplate builds a fresh session from a previous one, keeping exercises but no sets.
func FromTemplate(id string, template Workout, date shared.LocalDate, newID func() string) Workout {
	exercises := make([]Exercise, 0, len(template.Exercises))
	for _, e := range template.Exercises {
		exercises = append(exercises, Exercise{ID: newID(), Name: e.Name, Sets: []Set{}})
	}
	return Workout{ID: id, Name: template.Name, Date: date, Status: InProgress, Exercises: exercises}
}

// EstimatedOneRepMax is the Epley estimate. Bodyweight sets have no load and return 0.
func EstimatedOneRepMax(s Set) float64 {
	if s.Weight == nil || *s.Weight == 0 {
		return 0
	}
	return shared.Round(*s.Weight*(1+float64(s.Reps)/30), 1)
}

// ExerciseVolume is total kg lifted in an exercise.
func ExerciseVolume(e Exercise) float64 {
	total := 0.0
	for _, s := range e.Sets {
		if s.Weight != nil {
			total += *s.Weight * float64(s.Reps)
		}
	}
	return total
}

// Volume is total kg lifted in a workout.
func Volume(w Workout) float64 {
	total := 0.0
	for _, e := range w.Exercises {
		total += ExerciseVolume(e)
	}
	return total
}

// TotalSets counts the sets of a workout.
func TotalSets(w Workout) int {
	total := 0
	for _, e := range w.Exercises {
		total += len(e.Sets)
	}
	return total
}

// PersonalRecord is the best set of an exercise.
type PersonalRecord struct {
	Exercise           string           `json:"exercise"`
	Weight             *float64         `json:"weight"`
	Reps               int              `json:"reps"`
	EstimatedOneRepMax float64          `json:"estimatedOneRepMax"`
	Date               shared.LocalDate `json:"date"`
}

func loaded(s Set) bool { return s.Weight != nil && *s.Weight != 0 }

// PersonalRecords finds the best set per exercise across the history. Loaded
// lifts rank by estimated 1RM; bodyweight lifts rank by reps. Exercises keep
// the order in which they first appear.
func PersonalRecords(workouts []Workout) []PersonalRecord {
	sorted := slices.Clone(workouts)
	slices.SortStableFunc(sorted, func(a, b Workout) int { return strings.Compare(string(a.Date), string(b.Date)) })

	best := map[string]int{}
	records := []PersonalRecord{}
	for _, w := range sorted {
		for _, e := range w.Exercises {
			for _, s := range e.Sets {
				candidate := PersonalRecord{
					Exercise:           e.Name,
					Weight:             s.Weight,
					Reps:               s.Reps,
					EstimatedOneRepMax: EstimatedOneRepMax(s),
					Date:               w.Date,
				}
				i, ok := best[e.Name]
				if !ok {
					best[e.Name] = len(records)
					records = append(records, candidate)
					continue
				}
				current := records[i]
				better := candidate.Reps > current.Reps
				if loaded(s) {
					better = candidate.EstimatedOneRepMax > current.EstimatedOneRepMax
				}
				if better {
					records[i] = candidate
				}
			}
		}
	}
	return records
}

// IsRecordSet reports whether a set beats every earlier set of the same exercise.
func IsRecordSet(history []Workout, w Workout, exerciseName string, s Set) bool {
	earlier := []Workout{}
	for _, h := range history {
		if h.ID != w.ID && h.Date <= w.Date {
			earlier = append(earlier, h)
		}
	}
	for _, r := range PersonalRecords(earlier) {
		if r.Exercise != exerciseName {
			continue
		}
		if loaded(s) {
			return EstimatedOneRepMax(s) > r.EstimatedOneRepMax
		}
		return s.Reps > r.Reps
	}
	return false
}
