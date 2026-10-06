// Package training holds the workout use cases.
package training

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"ascend/internal/application/habits"
	"ascend/internal/application/ports"
	"ascend/internal/application/progression"
	"ascend/internal/domain/activity"
	"ascend/internal/domain/habit"
	"ascend/internal/domain/shared"
	"ascend/internal/domain/training"
)

// WeekVolume is total kg lifted in one ISO week.
type WeekVolume struct {
	Week     shared.LocalDate `json:"week"`
	Volume   float64          `json:"volume"`
	Sessions int              `json:"sessions"`
}

// Overview is the training screen at a glance.
type Overview struct {
	Current   *training.Workout         `json:"current"`
	History   []training.Workout        `json:"history"`
	Records   []training.PersonalRecord `json:"records"`
	Templates []string                  `json:"templates"`
	// WeeklyVolume covers the last 8 ISO weeks, oldest first.
	WeeklyVolume     []WeekVolume `json:"weeklyVolume"`
	SessionsThisWeek int          `json:"sessionsThisWeek"`
}

// LoggedSet reports whether the new set is a personal record.
type LoggedSet struct {
	Set      training.Set `json:"set"`
	IsRecord bool         `json:"isRecord"`
}

// Service exposes the workout use cases.
type Service struct{ p ports.Ports }

// NewService binds the workout use cases to ports.
func NewService(p ports.Ports) *Service { return &Service{p: p} }

func newestFirst(a, b training.Workout) int { return strings.Compare(string(b.Date), string(a.Date)) }

// Overview returns the running workout, history, records and weekly volume.
func (s *Service) Overview(ctx context.Context) (Overview, error) {
	today := s.p.Clock.Today()
	all, err := s.p.Workouts.GetAll(ctx)
	if err != nil {
		return Overview{}, err
	}
	slices.SortStableFunc(all, newestFirst)

	o := Overview{History: []training.Workout{}, Templates: []string{}}
	for _, w := range all {
		switch {
		case w.Status == training.Completed:
			o.History = append(o.History, w)
			if !slices.Contains(o.Templates, w.Name) {
				o.Templates = append(o.Templates, w.Name)
			}
		case o.Current == nil:
			current := w
			o.Current = &current
		}
	}
	slices.Sort(o.Templates)
	o.Records = training.PersonalRecords(o.History)
	slices.SortStableFunc(o.Records, func(a, b training.PersonalRecord) int {
		return cmp.Compare(b.EstimatedOneRepMax, a.EstimatedOneRepMax)
	})

	thisWeek := today.StartOfWeek()
	o.WeeklyVolume = make([]WeekVolume, 0, 8)
	for i := range 8 {
		week := thisWeek.AddDays((i - 7) * 7)
		end := week.AddDays(6)
		v := WeekVolume{Week: week}
		for _, w := range o.History {
			if w.Date.Within(week, end) {
				v.Volume += training.Volume(w)
				v.Sessions++
			}
		}
		o.WeeklyVolume = append(o.WeeklyVolume, v)
	}
	for _, w := range o.History {
		if w.Date >= thisWeek {
			o.SessionsThisWeek++
		}
	}
	return o, nil
}

// Get returns one workout.
func (s *Service) Get(ctx context.Context, id string) (training.Workout, error) {
	return ports.Require(ctx, s.p.Workouts, "Workout", id)
}

// Start begins a session using the most recent workout with that name as
// template, or an empty one when the name is new.
func (s *Service) Start(ctx context.Context, templateName string) (training.Workout, error) {
	return ports.InTx(ctx, s.p.Tx, func(ctx context.Context) (training.Workout, error) {
		all, err := s.p.Workouts.GetAll(ctx)
		if err != nil {
			return training.Workout{}, err
		}
		if slices.ContainsFunc(all, func(w training.Workout) bool { return w.Status == training.InProgress }) {
			return training.Workout{}, shared.Invalid("Finish the current workout first")
		}
		name := strings.TrimSpace(templateName)
		if len([]rune(name)) > 60 {
			return training.Workout{}, shared.Invalid("Keep the name under 60 characters")
		}
		candidates := slices.DeleteFunc(all, func(w training.Workout) bool { return w.Name != name })
		slices.SortStableFunc(candidates, newestFirst)

		today := s.p.Clock.Today()
		var w training.Workout
		if len(candidates) > 0 {
			w = training.FromTemplate(s.p.IDs.Next(), candidates[0], today, s.p.IDs.Next)
		} else {
			w = training.New(s.p.IDs.Next(), name, today)
		}
		return w, s.p.Workouts.Save(ctx, w)
	})
}

// LogSet records a set and reports whether it is a personal record.
func (s *Service) LogSet(ctx context.Context, workoutID, exerciseID string, in training.SetInput) (LoggedSet, error) {
	return ports.InTx(ctx, s.p.Tx, func(ctx context.Context) (LoggedSet, error) {
		w, err := ports.Require(ctx, s.p.Workouts, "Workout", workoutID)
		if err != nil {
			return LoggedSet{}, err
		}
		i := slices.IndexFunc(w.Exercises, func(e training.Exercise) bool { return e.ID == exerciseID })
		if i < 0 {
			return LoggedSet{}, shared.NotFound("Exercise", exerciseID)
		}
		set := training.Set{ID: s.p.IDs.Next(), Weight: in.Weight, Reps: in.Reps, RIR: in.RIR}
		updated, err := training.LogSet(w, exerciseID, set)
		if err != nil {
			return LoggedSet{}, err
		}
		if err := s.p.Workouts.Save(ctx, updated); err != nil {
			return LoggedSet{}, err
		}
		history, err := s.p.Workouts.GetAll(ctx)
		if err != nil {
			return LoggedSet{}, err
		}
		return LoggedSet{Set: set, IsRecord: training.IsRecordSet(history, w, w.Exercises[i].Name, set)}, nil
	})
}

// RemoveSet deletes a logged set.
func (s *Service) RemoveSet(ctx context.Context, workoutID, exerciseID, setID string) (training.Workout, error) {
	return ports.InTx(ctx, s.p.Tx, func(ctx context.Context) (training.Workout, error) {
		w, err := ports.Require(ctx, s.p.Workouts, "Workout", workoutID)
		if err != nil {
			return training.Workout{}, err
		}
		updated, err := training.RemoveSet(w, exerciseID, setID)
		if err != nil {
			return training.Workout{}, err
		}
		return updated, s.p.Workouts.Save(ctx, updated)
	})
}

// AddExercise appends an exercise to a running workout.
func (s *Service) AddExercise(ctx context.Context, workoutID, name string) (training.Workout, error) {
	return ports.InTx(ctx, s.p.Tx, func(ctx context.Context) (training.Workout, error) {
		w, err := ports.Require(ctx, s.p.Workouts, "Workout", workoutID)
		if err != nil {
			return training.Workout{}, err
		}
		updated, err := training.AddExercise(w, training.Exercise{ID: s.p.IDs.Next(), Name: name})
		if err != nil {
			return training.Workout{}, err
		}
		return updated, s.p.Workouts.Save(ctx, updated)
	})
}

// Finish completes the workout and the habit wired to training, or grants
// base XP when no habit is wired (or it was already done today).
func (s *Service) Finish(ctx context.Context, workoutID string, durationMinutes int) (habits.TriggeredReward, error) {
	return ports.InTx(ctx, s.p.Tx, func(ctx context.Context) (habits.TriggeredReward, error) {
		current, err := ports.Require(ctx, s.p.Workouts, "Workout", workoutID)
		if err != nil {
			return habits.TriggeredReward{}, err
		}
		w, err := training.Complete(current, durationMinutes)
		if err != nil {
			return habits.TriggeredReward{}, err
		}
		if err := s.p.Workouts.Save(ctx, w); err != nil {
			return habits.TriggeredReward{}, err
		}
		detail := fmt.Sprintf("%d sets · %s kg", training.TotalSets(w), shared.FormatNumber(training.Volume(w)))
		title := "Finished " + w.Name

		habitReward, err := habits.CompleteTriggered(ctx, s.p, habit.TriggerWorkout)
		if err != nil {
			return habits.TriggeredReward{}, err
		}
		if habitReward != nil {
			err := s.p.Activity.Append(ctx, activity.Event{
				ID:       s.p.IDs.Next(),
				Kind:     activity.KindWorkout,
				Title:    title,
				Detail:   detail,
				DomainID: training.DomainID,
				At:       s.p.Clock.Now().UTC(),
			})
			return *habitReward, err
		}
		reward, err := progression.GrantReward(ctx, s.p, progression.RewardInput{
			XP:       training.WorkoutXP,
			DomainID: training.DomainID,
			RefID:    "workout:" + w.ID,
			Kind:     activity.KindWorkout,
			Title:    title,
			Detail:   detail,
		})
		return habits.TriggeredReward{Reward: reward}, err
	})
}

// Discard deletes a workout that is still running.
func (s *Service) Discard(ctx context.Context, workoutID string) error {
	return s.p.Tx.WithinTx(ctx, func(ctx context.Context) error {
		w, err := ports.Require(ctx, s.p.Workouts, "Workout", workoutID)
		if err != nil {
			return err
		}
		if w.Status == training.Completed {
			return shared.Invalid("Completed workouts stay in history")
		}
		return s.p.Workouts.Delete(ctx, workoutID)
	})
}
