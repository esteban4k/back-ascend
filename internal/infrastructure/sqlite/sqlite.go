// Package sqlite implements every repository port on SQLite (pure Go driver,
// no cgo). Entities are stored as JSON documents, one table per collection,
// with the columns that queries filter on promoted next to them.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver

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

// Store is a SQLite database holding every collection.
type Store struct {
	db *sql.DB
}

// migrations run in order; each one is applied exactly once.
var migrations = []string{
	`CREATE TABLE users (id TEXT PRIMARY KEY, data TEXT NOT NULL);
	 CREATE TABLE domains (id TEXT PRIMARY KEY, data TEXT NOT NULL);
	 CREATE TABLE habits (id TEXT PRIMARY KEY, data TEXT NOT NULL);
	 CREATE TABLE objectives (id TEXT PRIMARY KEY, data TEXT NOT NULL);
	 CREATE TABLE tasks (id TEXT PRIMARY KEY, date TEXT NOT NULL, data TEXT NOT NULL);
	 CREATE INDEX tasks_date ON tasks (date);
	 CREATE TABLE journal (id TEXT PRIMARY KEY, data TEXT NOT NULL);
	 CREATE TABLE focus_sessions (id TEXT PRIMARY KEY, data TEXT NOT NULL);
	 CREATE TABLE workouts (id TEXT PRIMARY KEY, data TEXT NOT NULL);
	 CREATE TABLE journey (id TEXT PRIMARY KEY, data TEXT NOT NULL);
	 CREATE TABLE activity (id TEXT PRIMARY KEY, at TEXT NOT NULL, ref_id TEXT, data TEXT NOT NULL);
	 CREATE INDEX activity_at ON activity (at);
	 CREATE INDEX activity_ref ON activity (ref_id);`,
}

var tables = []string{"users", "domains", "habits", "objectives", "tasks", "journal", "focus_sessions", "workouts", "journey", "activity"}

// Open opens (or creates) the database at `path` and applies pending
// migrations. Use ":memory:" for a throwaway database.
func Open(ctx context.Context, path string) (*Store, error) {
	dsn := "file::memory:"
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("sqlite: create data directory: %w", err)
		}
		dsn = "file:" + filepath.ToSlash(path)
	}
	query := url.Values{}
	query.Add("_pragma", "busy_timeout(5000)")
	query.Add("_pragma", "journal_mode(WAL)")
	query.Add("_pragma", "synchronous(NORMAL)")
	query.Add("_txlock", "immediate")
	db, err := sql.Open("sqlite", dsn+"?"+query.Encode())
	if err != nil {
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}
	// One connection serializes writers (SQLite allows only one anyway) and
	// keeps ":memory:" databases alive across calls.
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(0)

	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY)`); err != nil {
		return fmt.Errorf("sqlite: migrations table: %w", err)
	}
	var applied int
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&applied); err != nil {
		return fmt.Errorf("sqlite: read schema version: %w", err)
	}
	for v := applied + 1; v <= len(migrations); v++ {
		err := s.WithinTx(ctx, func(ctx context.Context) error {
			if _, err := s.conn(ctx).ExecContext(ctx, migrations[v-1]); err != nil {
				return err
			}
			_, err := s.conn(ctx).ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES (?)`, v)
			return err
		})
		if err != nil {
			return fmt.Errorf("sqlite: migration %d: %w", v, err)
		}
	}
	return nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// Ping checks the database is reachable.
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

// ─── Units of work ──────────────────────────────────────────────────────────

type txKey struct{}

type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// conn returns the transaction bound to ctx, or the database.
func (s *Store) conn(ctx context.Context) querier {
	if tx, ok := ctx.Value(txKey{}).(*sql.Tx); ok {
		return tx
	}
	return s.db
}

// WithinTx runs fn in a database transaction. Nested calls join the outer one.
func (s *Store) WithinTx(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	if _, ok := ctx.Value(txKey{}).(*sql.Tx); ok {
		return fn(ctx)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: begin: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()
	if err := fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit: %w", err)
	}
	return nil
}

// ─── Loading ────────────────────────────────────────────────────────────────

// Empty reports whether the database has no account yet.
func (s *Store) Empty(ctx context.Context) (bool, error) {
	var n int
	if err := s.conn(ctx).QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return false, fmt.Errorf("sqlite: count users: %w", err)
	}
	return n == 0, nil
}

// Load replaces every collection with `data`, atomically.
func (s *Store) Load(ctx context.Context, data seed.Dataset) error {
	p := s.Ports(nil, nil)
	return s.WithinTx(ctx, func(ctx context.Context) error {
		for _, t := range tables {
			if _, err := s.conn(ctx).ExecContext(ctx, "DELETE FROM "+t); err != nil {
				return fmt.Errorf("sqlite: clear %s: %w", t, err)
			}
		}
		if err := p.Users.Save(ctx, data.User); err != nil {
			return err
		}
		if err := saveAll(ctx, p.Domains, data.Domains); err != nil {
			return err
		}
		if err := saveAll(ctx, p.Habits, data.Habits); err != nil {
			return err
		}
		if err := saveAll(ctx, p.Objectives, data.Objectives); err != nil {
			return err
		}
		if err := saveAll[task.Task](ctx, p.Tasks, data.Tasks); err != nil {
			return err
		}
		if err := saveAll(ctx, p.Journal, data.Journal); err != nil {
			return err
		}
		if err := saveAll(ctx, p.Focus, data.Focus); err != nil {
			return err
		}
		if err := saveAll(ctx, p.Workouts, data.Workouts); err != nil {
			return err
		}
		if err := saveAll(ctx, p.Journey, data.Journey); err != nil {
			return err
		}
		for _, e := range data.Activity {
			if err := p.Activity.Append(ctx, e); err != nil {
				return err
			}
		}
		return nil
	})
}

func saveAll[T any](ctx context.Context, repo ports.Repository[T], items []T) error {
	for _, item := range items {
		if err := repo.Save(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

// Ports wires the repositories to the given clock and id generator.
func (s *Store) Ports(clock ports.Clock, ids ports.IDGenerator) ports.Ports {
	return ports.Ports{
		Habits:     &docs[habit.Habit]{s: s, table: "habits", id: func(h habit.Habit) string { return h.ID }},
		Objectives: &docs[objective.Objective]{s: s, table: "objectives", id: func(o objective.Objective) string { return o.ID }},
		Tasks:      &taskRepository{docs[task.Task]{s: s, table: "tasks", id: func(t task.Task) string { return t.ID }, extra: taskColumns}},
		Domains:    &docs[lifedomain.LifeDomain]{s: s, table: "domains", id: func(d lifedomain.LifeDomain) string { return d.ID }},
		Journal:    &docs[journal.Entry]{s: s, table: "journal", id: func(e journal.Entry) string { return e.ID }},
		Focus:      &docs[focus.Session]{s: s, table: "focus_sessions", id: func(f focus.Session) string { return f.ID }},
		Workouts:   &docs[training.Workout]{s: s, table: "workouts", id: func(w training.Workout) string { return w.ID }},
		Journey:    &docs[journey.Milestone]{s: s, table: "journey", id: func(m journey.Milestone) string { return m.ID }},
		Activity:   &activityRepository{s: s},
		Users:      &userRepository{s: s},
		Clock:      clock,
		IDs:        ids,
		Tx:         s,
	}
}

// ─── Generic document collection ────────────────────────────────────────────

type column struct {
	name  string
	value any
}

type docs[T any] struct {
	s     *Store
	table string
	id    func(T) string
	extra func(T) []column
}

func scanAll[T any](rows *sql.Rows) ([]T, error) {
	defer rows.Close()
	out := []T{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var item T
		if err := json.Unmarshal([]byte(raw), &item); err != nil {
			return nil, fmt.Errorf("sqlite: decode row: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *docs[T]) GetAll(ctx context.Context) ([]T, error) {
	rows, err := r.s.conn(ctx).QueryContext(ctx, "SELECT data FROM "+r.table+" ORDER BY rowid")
	if err != nil {
		return nil, fmt.Errorf("sqlite: list %s: %w", r.table, err)
	}
	return scanAll[T](rows)
}

func (r *docs[T]) GetByID(ctx context.Context, id string) (*T, error) {
	var raw string
	err := r.s.conn(ctx).QueryRowContext(ctx, "SELECT data FROM "+r.table+" WHERE id = ?", id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: get %s %s: %w", r.table, id, err)
	}
	var item T
	if err := json.Unmarshal([]byte(raw), &item); err != nil {
		return nil, fmt.Errorf("sqlite: decode %s %s: %w", r.table, id, err)
	}
	return &item, nil
}

func (r *docs[T]) Save(ctx context.Context, entity T) error {
	raw, err := json.Marshal(entity)
	if err != nil {
		return fmt.Errorf("sqlite: encode %s: %w", r.table, err)
	}
	cols := "id, data"
	marks := "?, ?"
	updates := "data = excluded.data"
	args := []any{r.id(entity), string(raw)}
	if r.extra != nil {
		for _, c := range r.extra(entity) {
			cols += ", " + c.name
			marks += ", ?"
			updates += ", " + c.name + " = excluded." + c.name
			args = append(args, c.value)
		}
	}
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) ON CONFLICT(id) DO UPDATE SET %s", r.table, cols, marks, updates)
	if _, err := r.s.conn(ctx).ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("sqlite: save %s: %w", r.table, err)
	}
	return nil
}

func (r *docs[T]) Delete(ctx context.Context, id string) error {
	if _, err := r.s.conn(ctx).ExecContext(ctx, "DELETE FROM "+r.table+" WHERE id = ?", id); err != nil {
		return fmt.Errorf("sqlite: delete %s %s: %w", r.table, id, err)
	}
	return nil
}

// ─── Tasks ──────────────────────────────────────────────────────────────────

func taskColumns(t task.Task) []column { return []column{{"date", string(t.Date)}} }

type taskRepository struct {
	docs[task.Task]
}

func (r *taskRepository) GetBetween(ctx context.Context, from, to shared.LocalDate) ([]task.Task, error) {
	rows, err := r.s.conn(ctx).QueryContext(ctx, "SELECT data FROM tasks WHERE date BETWEEN ? AND ? ORDER BY rowid", string(from), string(to))
	if err != nil {
		return nil, fmt.Errorf("sqlite: tasks between: %w", err)
	}
	return scanAll[task.Task](rows)
}

// ─── Activity ───────────────────────────────────────────────────────────────

// instantLayout sorts lexically in chronological order.
const instantLayout = "2006-01-02T15:04:05.000Z"

func instant(t time.Time) string { return t.UTC().Format(instantLayout) }

type activityRepository struct{ s *Store }

func (r *activityRepository) List(ctx context.Context, q ports.ActivityQuery) ([]activity.Event, error) {
	query := "SELECT data FROM activity"
	args := []any{}
	if !q.Since.IsZero() {
		query += " WHERE at >= ?"
		args = append(args, instant(q.Since))
	}
	query += " ORDER BY at DESC, rowid ASC"
	if q.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, q.Limit)
	}
	rows, err := r.s.conn(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list activity: %w", err)
	}
	return scanAll[activity.Event](rows)
}

func (r *activityRepository) Append(ctx context.Context, e activity.Event) error {
	raw, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("sqlite: encode activity: %w", err)
	}
	var ref any
	if e.RefID != "" {
		ref = e.RefID
	}
	_, err = r.s.conn(ctx).ExecContext(ctx, "INSERT INTO activity (id, at, ref_id, data) VALUES (?, ?, ?, ?)", e.ID, instant(e.At), ref, string(raw))
	if err != nil {
		return fmt.Errorf("sqlite: append activity: %w", err)
	}
	return nil
}

func (r *activityRepository) FindByRef(ctx context.Context, refID string) (*activity.Event, error) {
	rows, err := r.s.conn(ctx).QueryContext(ctx, "SELECT data FROM activity WHERE ref_id = ? ORDER BY rowid LIMIT 1", refID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: find activity: %w", err)
	}
	found, err := scanAll[activity.Event](rows)
	if err != nil || len(found) == 0 {
		return nil, err
	}
	return &found[0], nil
}

func (r *activityRepository) Delete(ctx context.Context, id string) error {
	if _, err := r.s.conn(ctx).ExecContext(ctx, "DELETE FROM activity WHERE id = ?", id); err != nil {
		return fmt.Errorf("sqlite: delete activity: %w", err)
	}
	return nil
}

// ─── User ───────────────────────────────────────────────────────────────────

// ErrNoUser is returned when the database has not been initialized.
var ErrNoUser = errors.New("sqlite: no user; load a dataset first")

type userRepository struct{ s *Store }

func (r *userRepository) GetCurrent(ctx context.Context) (progression.User, error) {
	rows, err := r.s.conn(ctx).QueryContext(ctx, "SELECT data FROM users ORDER BY rowid LIMIT 1")
	if err != nil {
		return progression.User{}, fmt.Errorf("sqlite: get user: %w", err)
	}
	users, err := scanAll[progression.User](rows)
	if err != nil {
		return progression.User{}, err
	}
	if len(users) == 0 {
		return progression.User{}, ErrNoUser
	}
	return users[0], nil
}

func (r *userRepository) Save(ctx context.Context, u progression.User) error {
	return (&docs[progression.User]{s: r.s, table: "users", id: func(u progression.User) string { return u.ID }}).Save(ctx, u)
}
