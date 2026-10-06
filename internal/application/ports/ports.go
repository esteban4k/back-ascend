// Package ports declares what the application needs from the outside world.
// Adapters in `infrastructure/` implement these interfaces (SQLite and
// in-memory today); use cases only ever see these types.
package ports

import (
	"context"
	"time"

	"ascend/internal/domain/activity"
	"ascend/internal/domain/focus"
	"ascend/internal/domain/habit"
	"ascend/internal/domain/journal"
	"ascend/internal/domain/journey"
	"ascend/internal/domain/lifedomain"
	"ascend/internal/domain/objective"
	"ascend/internal/domain/progression"
	"ascend/internal/domain/shared"
	"ascend/internal/domain/task"
	"ascend/internal/domain/training"
)

// Repository is the collection contract shared by every entity.
// GetAll returns entities in insertion order. GetByID returns nil, nil when
// the entity does not exist.
type Repository[T any] interface {
	GetAll(ctx context.Context) ([]T, error)
	GetByID(ctx context.Context, id string) (*T, error)
	Save(ctx context.Context, entity T) error
	Delete(ctx context.Context, id string) error
}

type (
	HabitRepository      = Repository[habit.Habit]
	ObjectiveRepository  = Repository[objective.Objective]
	LifeDomainRepository = Repository[lifedomain.LifeDomain]
	JournalRepository    = Repository[journal.Entry]
	FocusRepository      = Repository[focus.Session]
	WorkoutRepository    = Repository[training.Workout]
	JourneyRepository    = Repository[journey.Milestone]
)

// TaskRepository adds a date-range query to the base contract.
type TaskRepository interface {
	Repository[task.Task]
	GetBetween(ctx context.Context, from, to shared.LocalDate) ([]task.Task, error)
}

// ActivityQuery filters the activity log. Zero values mean "no filter".
type ActivityQuery struct {
	Limit int
	Since time.Time
}

// ActivityRepository is the append-only activity log.
type ActivityRepository interface {
	// List returns events newest first.
	List(ctx context.Context, q ActivityQuery) ([]activity.Event, error)
	Append(ctx context.Context, e activity.Event) error
	// FindByRef returns the oldest event with that refId, or nil.
	FindByRef(ctx context.Context, refID string) (*activity.Event, error)
	Delete(ctx context.Context, id string) error
}

// UserRepository holds the account being used.
type UserRepository interface {
	GetCurrent(ctx context.Context) (progression.User, error)
	Save(ctx context.Context, u progression.User) error
}

// Clock is the source of time. Injected so use cases are deterministic under test.
type Clock interface {
	// Now returns the current instant in the user's location.
	Now() time.Time
	// Today returns the current calendar date in the user's location.
	Today() shared.LocalDate
	// Location is the user's timezone, used to turn instants into dates.
	Location() *time.Location
}

// IDGenerator creates unique identifiers.
type IDGenerator interface {
	Next() string
}

// Transactor runs a unit of work atomically: every write inside `fn` is
// committed together or not at all. Nested calls join the outer unit.
type Transactor interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// Ports is everything the application layer depends on. Built once in the
// composition root.
type Ports struct {
	Habits     HabitRepository
	Objectives ObjectiveRepository
	Tasks      TaskRepository
	Domains    LifeDomainRepository
	Journal    JournalRepository
	Focus      FocusRepository
	Workouts   WorkoutRepository
	Journey    JourneyRepository
	Activity   ActivityRepository
	Users      UserRepository
	Clock      Clock
	IDs        IDGenerator
	Tx         Transactor
}

// InTx runs fn inside a unit of work and returns its result.
func InTx[T any](ctx context.Context, tx Transactor, fn func(ctx context.Context) (T, error)) (T, error) {
	var out T
	err := tx.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		out, err = fn(ctx)
		return err
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return out, nil
}

// DateOf returns the calendar date of an instant in the clock's location.
func DateOf(c Clock, t time.Time) shared.LocalDate {
	return shared.DateOf(t.In(c.Location()))
}

// Require loads an entity or fails with a NotFoundError naming it.
func Require[T any](ctx context.Context, repo Repository[T], entity, id string) (T, error) {
	found, err := repo.GetByID(ctx, id)
	if err != nil {
		var zero T
		return zero, err
	}
	if found == nil {
		var zero T
		return zero, shared.NotFound(entity, id)
	}
	return *found, nil
}
