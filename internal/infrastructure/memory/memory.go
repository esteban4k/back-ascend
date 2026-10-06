// Package memory is an in-memory implementation of every repository port.
// It backs the test suite and `ASCEND_STORE=memory`, the same role the mock
// adapters play in the web client. Data lives only as long as the process.
package memory

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"

	"ascend/internal/application/ports"
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
	"ascend/internal/infrastructure/seed"
)

// Store holds every collection.
//
// Writes are copy-on-write: a collection slice is never mutated in place,
// only replaced. That makes a shallow copy of the state a valid snapshot,
// which is how a failed unit of work is rolled back.
type Store struct {
	mu    sync.RWMutex // guards state
	txMu  sync.Mutex   // serializes units of work
	state seed.Dataset
}

// New returns an empty store.
func New() *Store { return &Store{} }

// clone deep-copies a value so callers can never mutate stored state by
// accident, exactly like data coming back from a database.
func clone[T any](v T) T {
	raw, err := json.Marshal(v)
	if err != nil {
		panic("memory: clone: " + err.Error())
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		panic("memory: clone: " + err.Error())
	}
	return out
}

// Load replaces every collection with `data`.
func (s *Store) Load(_ context.Context, data seed.Dataset) error {
	s.txMu.Lock()
	defer s.txMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = clone(data)
	return nil
}

// Empty reports whether the store has no account yet.
func (s *Store) Empty(context.Context) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.User.ID == "", nil
}

// Close is a no-op.
func (s *Store) Close() error { return nil }

// Ports wires the repositories to the given clock and id generator.
func (s *Store) Ports(clock ports.Clock, ids ports.IDGenerator) ports.Ports {
	return ports.Ports{
		Habits:     &repository[habit.Habit]{s, func(d *seed.Dataset) *[]habit.Habit { return &d.Habits }, func(h habit.Habit) string { return h.ID }},
		Objectives: &repository[objective.Objective]{s, func(d *seed.Dataset) *[]objective.Objective { return &d.Objectives }, func(o objective.Objective) string { return o.ID }},
		Tasks:      &taskRepository{repository[task.Task]{s, func(d *seed.Dataset) *[]task.Task { return &d.Tasks }, func(t task.Task) string { return t.ID }}},
		Domains:    &repository[lifedomain.LifeDomain]{s, func(d *seed.Dataset) *[]lifedomain.LifeDomain { return &d.Domains }, func(l lifedomain.LifeDomain) string { return l.ID }},
		Journal:    &repository[journal.Entry]{s, func(d *seed.Dataset) *[]journal.Entry { return &d.Journal }, func(e journal.Entry) string { return e.ID }},
		Focus:      &repository[focus.Session]{s, func(d *seed.Dataset) *[]focus.Session { return &d.Focus }, func(f focus.Session) string { return f.ID }},
		Workouts:   &repository[training.Workout]{s, func(d *seed.Dataset) *[]training.Workout { return &d.Workouts }, func(w training.Workout) string { return w.ID }},
		Journey:    &repository[journey.Milestone]{s, func(d *seed.Dataset) *[]journey.Milestone { return &d.Journey }, func(m journey.Milestone) string { return m.ID }},
		Activity:   &activityRepository{s},
		Users:      &userRepository{s},
		Clock:      clock,
		IDs:        ids,
		Tx:         s,
	}
}

type txKey struct{}

// WithinTx runs fn as one unit of work. Units of work are serialized; if fn
// fails, every write it made is rolled back. Nested calls join the outer unit.
func (s *Store) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if ctx.Value(txKey{}) == s {
		return fn(ctx)
	}
	s.txMu.Lock()
	defer s.txMu.Unlock()

	s.mu.RLock()
	snapshot := s.state
	s.mu.RUnlock()

	if err := fn(context.WithValue(ctx, txKey{}, s)); err != nil {
		s.mu.Lock()
		s.state = snapshot
		s.mu.Unlock()
		return err
	}
	return nil
}

// ─── Generic collection ─────────────────────────────────────────────────────

type repository[T any] struct {
	s   *Store
	col func(*seed.Dataset) *[]T
	id  func(T) string
}

func (r *repository[T]) items() []T {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()
	return *r.col(&r.s.state)
}

func (r *repository[T]) GetAll(context.Context) ([]T, error) {
	items := r.items()
	if items == nil {
		return []T{}, nil
	}
	return clone(items), nil
}

func (r *repository[T]) GetByID(_ context.Context, id string) (*T, error) {
	for _, item := range r.items() {
		if r.id(item) == id {
			found := clone(item)
			return &found, nil
		}
	}
	return nil, nil
}

func (r *repository[T]) Save(_ context.Context, entity T) error {
	stored := clone(entity)
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	col := r.col(&r.s.state)
	next := slices.Clone(*col)
	if i := slices.IndexFunc(next, func(item T) bool { return r.id(item) == r.id(entity) }); i >= 0 {
		next[i] = stored
	} else {
		next = append(next, stored)
	}
	*col = next
	return nil
}

func (r *repository[T]) Delete(_ context.Context, id string) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	col := r.col(&r.s.state)
	*col = slices.DeleteFunc(slices.Clone(*col), func(item T) bool { return r.id(item) == id })
	return nil
}

// ─── Tasks ──────────────────────────────────────────────────────────────────

type taskRepository struct {
	repository[task.Task]
}

func (r *taskRepository) GetBetween(_ context.Context, from, to shared.LocalDate) ([]task.Task, error) {
	out := []task.Task{}
	for _, t := range r.items() {
		if t.Date.Within(from, to) {
			out = append(out, clone(t))
		}
	}
	return out, nil
}

// ─── Activity ───────────────────────────────────────────────────────────────

type activityRepository struct{ s *Store }

func (r *activityRepository) List(_ context.Context, q ports.ActivityQuery) ([]activity.Event, error) {
	r.s.mu.RLock()
	events := slices.Clone(r.s.state.Activity)
	r.s.mu.RUnlock()

	slices.SortStableFunc(events, func(a, b activity.Event) int { return cmp.Compare(b.At.UnixMilli(), a.At.UnixMilli()) })
	if !q.Since.IsZero() {
		events = slices.DeleteFunc(events, func(e activity.Event) bool { return e.At.Before(q.Since) })
	}
	if q.Limit > 0 && len(events) > q.Limit {
		events = events[:q.Limit]
	}
	if len(events) == 0 {
		return []activity.Event{}, nil
	}
	return clone(events), nil
}

func (r *activityRepository) Append(_ context.Context, e activity.Event) error {
	stored := clone(e)
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	r.s.state.Activity = append(slices.Clone(r.s.state.Activity), stored)
	return nil
}

func (r *activityRepository) FindByRef(_ context.Context, refID string) (*activity.Event, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()
	for _, e := range r.s.state.Activity {
		if e.RefID == refID {
			found := clone(e)
			return &found, nil
		}
	}
	return nil, nil
}

func (r *activityRepository) Delete(_ context.Context, id string) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	r.s.state.Activity = slices.DeleteFunc(slices.Clone(r.s.state.Activity), func(e activity.Event) bool { return e.ID == id })
	return nil
}

// ─── User ───────────────────────────────────────────────────────────────────

type userRepository struct{ s *Store }

// ErrNoUser is returned when the store has not been initialized.
var ErrNoUser = errors.New("memory: no user; load a dataset first")

func (r *userRepository) GetCurrent(context.Context) (progression.User, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()
	if r.s.state.User.ID == "" {
		return progression.User{}, ErrNoUser
	}
	return r.s.state.User, nil
}

func (r *userRepository) Save(_ context.Context, u progression.User) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	r.s.state.User = u
	return nil
}
