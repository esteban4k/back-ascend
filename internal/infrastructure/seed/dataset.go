// Package seed builds initial data: a realistic demo account, or a blank one.
package seed

import (
	"ascend/internal/domain/activity"
	"ascend/internal/domain/focus"
	"ascend/internal/domain/habit"
	"ascend/internal/domain/journal"
	"ascend/internal/domain/journey"
	"ascend/internal/domain/lifedomain"
	"ascend/internal/domain/objective"
	"ascend/internal/domain/progression"
	"ascend/internal/domain/task"
	"ascend/internal/domain/training"
)

// Dataset is the full content of a store, used to load or reset it.
type Dataset struct {
	User       progression.User        `json:"user"`
	Domains    []lifedomain.LifeDomain `json:"domains"`
	Habits     []habit.Habit           `json:"habits"`
	Objectives []objective.Objective   `json:"objectives"`
	Tasks      []task.Task             `json:"tasks"`
	Journal    []journal.Entry         `json:"journal"`
	Focus      []focus.Session         `json:"focus"`
	Workouts   []training.Workout      `json:"workouts"`
	Journey    []journey.Milestone     `json:"journey"`
	Activity   []activity.Event        `json:"activity"`
}
