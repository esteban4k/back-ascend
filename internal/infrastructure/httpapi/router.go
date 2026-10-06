// Package httpapi is the REST delivery adapter. It translates HTTP requests
// into use case calls and results into JSON; it holds no business rules.
package httpapi

import (
	"context"
	"log/slog"
	"math"
	"net/http"
	"time"

	"ascend/internal/application"
	"ascend/internal/domain/focus"
	"ascend/internal/domain/habit"
	"ascend/internal/domain/journal"
	"ascend/internal/domain/lifedomain"
	"ascend/internal/domain/objective"
	"ascend/internal/domain/shared"
	"ascend/internal/domain/task"
	"ascend/internal/domain/training"
)

// Options configures the HTTP adapter.
type Options struct {
	// AllowedOrigins may call the API from a browser ("*" for any).
	AllowedOrigins []string
	// APIToken, when set, is required as `Authorization: Bearer <token>`.
	APIToken string
	// Reset restores the initial data. Nil disables `POST /api/demo/reset`.
	Reset func(ctx context.Context) error
	// DataSource describes the storage in use, reported by the health check.
	DataSource string
	// Ping checks the storage is reachable. Optional.
	Ping   func(ctx context.Context) error
	Logger *slog.Logger
}

type api struct {
	uc   *application.UseCases
	opts Options
}

// NewHandler builds the HTTP handler with every route and middleware.
func NewHandler(uc *application.UseCases, opts Options) http.Handler {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	a := &api{uc: uc, opts: opts}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", a.health)
	mux.HandleFunc("POST /api/demo/reset", noContent(a.reset))

	// Profile & activity
	mux.HandleFunc("GET /api/profile", ok(func(r *http.Request) (any, error) { return a.uc.Profile.Get(r.Context()) }))
	mux.HandleFunc("PATCH /api/profile", okBody(func(r *http.Request, b struct {
		Name string `json:"name"`
	}) (any, error) {
		return a.uc.Profile.Rename(r.Context(), b.Name)
	}))
	mux.HandleFunc("GET /api/activity", ok(func(r *http.Request) (any, error) {
		limit, err := queryInt(r, "limit", 0)
		if err != nil {
			return nil, err
		}
		return a.uc.Profile.Activity(r.Context(), limit)
	}))

	// Today & tasks
	mux.HandleFunc("GET /api/today", ok(func(r *http.Request) (any, error) { return a.uc.Today.Get(r.Context()) }))
	mux.HandleFunc("GET /api/tasks", ok(func(r *http.Request) (any, error) {
		q := r.URL.Query()
		return a.uc.Today.List(r.Context(), shared.LocalDate(q.Get("from")), shared.LocalDate(q.Get("to")))
	}))
	mux.HandleFunc("POST /api/tasks", createdBody(func(r *http.Request, d task.Draft) (any, error) {
		return a.uc.Today.CreateTask(r.Context(), d)
	}))
	mux.HandleFunc("POST /api/tasks/{id}/complete", ok(func(r *http.Request) (any, error) {
		return a.uc.Today.CompleteTask(r.Context(), r.PathValue("id"))
	}))
	mux.HandleFunc("POST /api/tasks/{id}/reopen", ok(func(r *http.Request) (any, error) {
		return a.uc.Today.ReopenTask(r.Context(), r.PathValue("id"))
	}))
	mux.HandleFunc("DELETE /api/tasks/{id}", noContent(func(r *http.Request) error {
		return a.uc.Today.DeleteTask(r.Context(), r.PathValue("id"))
	}))

	// Habits
	mux.HandleFunc("GET /api/habits", ok(func(r *http.Request) (any, error) { return a.uc.Habits.List(r.Context()) }))
	mux.HandleFunc("GET /api/habits/consistency", ok(func(r *http.Request) (any, error) {
		days, err := queryInt(r, "days", 140)
		if err != nil {
			return nil, err
		}
		return a.uc.Habits.Consistency(r.Context(), days, r.URL.Query().Get("habitId"))
	}))
	mux.HandleFunc("GET /api/habits/{id}", ok(func(r *http.Request) (any, error) {
		return a.uc.Habits.Get(r.Context(), r.PathValue("id"))
	}))
	mux.HandleFunc("POST /api/habits", createdBody(func(r *http.Request, d habit.Draft) (any, error) {
		return a.uc.Habits.Create(r.Context(), d)
	}))
	mux.HandleFunc("PUT /api/habits/{id}", okBody(func(r *http.Request, d habit.Draft) (any, error) {
		return a.uc.Habits.Update(r.Context(), r.PathValue("id"), d)
	}))
	mux.HandleFunc("DELETE /api/habits/{id}", noContent(func(r *http.Request) error {
		return a.uc.Habits.Delete(r.Context(), r.PathValue("id"))
	}))
	mux.HandleFunc("POST /api/habits/{id}/complete", ok(func(r *http.Request) (any, error) {
		return a.uc.Habits.Complete(r.Context(), r.PathValue("id"))
	}))
	mux.HandleFunc("POST /api/habits/{id}/uncomplete", ok(func(r *http.Request) (any, error) {
		return a.uc.Habits.Uncomplete(r.Context(), r.PathValue("id"))
	}))

	// Objectives
	mux.HandleFunc("GET /api/objectives", ok(func(r *http.Request) (any, error) { return a.uc.Objectives.List(r.Context()) }))
	mux.HandleFunc("GET /api/objectives/current", ok(func(r *http.Request) (any, error) { return a.uc.Objectives.Current(r.Context()) }))
	mux.HandleFunc("GET /api/objectives/{id}", ok(func(r *http.Request) (any, error) {
		return a.uc.Objectives.Get(r.Context(), r.PathValue("id"))
	}))
	mux.HandleFunc("POST /api/objectives", createdBody(func(r *http.Request, d objective.Draft) (any, error) {
		return a.uc.Objectives.Create(r.Context(), d)
	}))
	mux.HandleFunc("POST /api/objectives/{id}/value", okBody(func(r *http.Request, b struct {
		Value *float64 `json:"value"`
	}) (any, error) {
		if b.Value == nil {
			return nil, badRequestf("value is required")
		}
		return a.uc.Objectives.RecordValue(r.Context(), r.PathValue("id"), *b.Value)
	}))
	mux.HandleFunc("POST /api/objectives/{id}/milestones/{milestoneId}/toggle", ok(func(r *http.Request) (any, error) {
		return a.uc.Objectives.ToggleMilestone(r.Context(), r.PathValue("id"), r.PathValue("milestoneId"))
	}))
	mux.HandleFunc("PUT /api/objectives/{id}/status", okBody(func(r *http.Request, b struct {
		Status objective.Status `json:"status"`
	}) (any, error) {
		return a.uc.Objectives.SetStatus(r.Context(), r.PathValue("id"), b.Status)
	}))
	mux.HandleFunc("DELETE /api/objectives/{id}", noContent(func(r *http.Request) error {
		return a.uc.Objectives.Delete(r.Context(), r.PathValue("id"))
	}))

	// Life domains
	mux.HandleFunc("GET /api/domains", ok(func(r *http.Request) (any, error) { return a.uc.Domains.List(r.Context()) }))
	mux.HandleFunc("GET /api/domains/options", ok(func(r *http.Request) (any, error) { return a.uc.Domains.Options(r.Context()) }))
	mux.HandleFunc("POST /api/domains", createdBody(func(r *http.Request, d lifedomain.Draft) (any, error) {
		return a.uc.Domains.Create(r.Context(), d)
	}))
	mux.HandleFunc("PUT /api/domains/{id}", okBody(func(r *http.Request, d lifedomain.Draft) (any, error) {
		return a.uc.Domains.Update(r.Context(), r.PathValue("id"), d)
	}))
	mux.HandleFunc("DELETE /api/domains/{id}", noContent(func(r *http.Request) error {
		return a.uc.Domains.Delete(r.Context(), r.PathValue("id"))
	}))

	// Journal
	mux.HandleFunc("GET /api/journal", ok(func(r *http.Request) (any, error) { return a.uc.Journal.List(r.Context()) }))
	mux.HandleFunc("GET /api/journal/{id}", ok(func(r *http.Request) (any, error) {
		return a.uc.Journal.Get(r.Context(), r.PathValue("id"))
	}))
	mux.HandleFunc("POST /api/journal", createdBody(func(r *http.Request, d journal.Draft) (any, error) {
		return a.uc.Journal.Create(r.Context(), d)
	}))
	mux.HandleFunc("PUT /api/journal/{id}", okBody(func(r *http.Request, d journal.Draft) (any, error) {
		return a.uc.Journal.Update(r.Context(), r.PathValue("id"), d)
	}))
	mux.HandleFunc("DELETE /api/journal/{id}", noContent(func(r *http.Request) error {
		return a.uc.Journal.Delete(r.Context(), r.PathValue("id"))
	}))

	// Focus
	mux.HandleFunc("GET /api/focus", ok(func(r *http.Request) (any, error) { return a.uc.Focus.Overview(r.Context()) }))
	mux.HandleFunc("POST /api/focus/timer", okBody(func(r *http.Request, in focus.TimerInput) (any, error) {
		return a.uc.Focus.Start(r.Context(), in)
	}))
	mux.HandleFunc("POST /api/focus/sessions", createdBody(func(r *http.Request, t focus.Timer) (any, error) {
		return a.uc.Focus.Complete(r.Context(), t)
	}))

	// Training
	mux.HandleFunc("GET /api/training", ok(func(r *http.Request) (any, error) { return a.uc.Training.Overview(r.Context()) }))
	mux.HandleFunc("GET /api/workouts/{id}", ok(func(r *http.Request) (any, error) {
		return a.uc.Training.Get(r.Context(), r.PathValue("id"))
	}))
	mux.HandleFunc("POST /api/workouts", createdBody(func(r *http.Request, b struct {
		Template string `json:"template"`
	}) (any, error) {
		return a.uc.Training.Start(r.Context(), b.Template)
	}))
	mux.HandleFunc("DELETE /api/workouts/{id}", noContent(func(r *http.Request) error {
		return a.uc.Training.Discard(r.Context(), r.PathValue("id"))
	}))
	mux.HandleFunc("POST /api/workouts/{id}/exercises", createdBody(func(r *http.Request, b struct {
		Name string `json:"name"`
	}) (any, error) {
		return a.uc.Training.AddExercise(r.Context(), r.PathValue("id"), b.Name)
	}))
	mux.HandleFunc("POST /api/workouts/{id}/exercises/{exerciseId}/sets", createdBody(func(r *http.Request, b setBody) (any, error) {
		in, err := b.toInput()
		if err != nil {
			return nil, err
		}
		return a.uc.Training.LogSet(r.Context(), r.PathValue("id"), r.PathValue("exerciseId"), in)
	}))
	mux.HandleFunc("DELETE /api/workouts/{id}/exercises/{exerciseId}/sets/{setId}", ok(func(r *http.Request) (any, error) {
		return a.uc.Training.RemoveSet(r.Context(), r.PathValue("id"), r.PathValue("exerciseId"), r.PathValue("setId"))
	}))
	mux.HandleFunc("POST /api/workouts/{id}/finish", okBody(func(r *http.Request, b struct {
		DurationMinutes int `json:"durationMinutes"`
	}) (any, error) {
		return a.uc.Training.Finish(r.Context(), r.PathValue("id"), b.DurationMinutes)
	}))

	// Calendar, performance, journey
	mux.HandleFunc("GET /api/calendar", ok(func(r *http.Request) (any, error) {
		q := r.URL.Query()
		return a.uc.Calendar.Agenda(r.Context(), shared.LocalDate(q.Get("from")), shared.LocalDate(q.Get("to")))
	}))
	mux.HandleFunc("GET /api/performance", ok(func(r *http.Request) (any, error) { return a.uc.Performance.Report(r.Context()) }))
	mux.HandleFunc("GET /api/journey", ok(func(r *http.Request) (any, error) { return a.uc.Journey.Get(r.Context()) }))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "No route for "+r.Method+" "+r.URL.Path)
	})

	var h http.Handler = mux
	h = requireToken(opts.APIToken, h)
	h = cors(opts.AllowedOrigins, h)
	h = logRequests(opts.Logger, h)
	h = recoverPanics(opts.Logger, h)
	return h
}

func (a *api) health(w http.ResponseWriter, r *http.Request) {
	status := http.StatusOK
	state := "ok"
	if a.opts.Ping != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := a.opts.Ping(ctx); err != nil {
			status, state = http.StatusServiceUnavailable, "unavailable"
		}
	}
	writeJSON(w, status, map[string]string{"status": state, "dataSource": a.opts.DataSource})
}

func (a *api) reset(r *http.Request) error {
	if a.opts.Reset == nil {
		return shared.NotFound("Route", "POST /api/demo/reset")
	}
	return a.opts.Reset(r.Context())
}

// setBody accepts numbers as the web client sends them and checks that
// counts are whole before handing them to the domain.
type setBody struct {
	Weight *float64 `json:"weight"`
	Reps   float64  `json:"reps"`
	RIR    *float64 `json:"rir"`
}

func whole(v float64) bool { return v == math.Trunc(v) && !math.IsInf(v, 0) && math.Abs(v) < 1e6 }

func (b setBody) toInput() (training.SetInput, error) {
	if !whole(b.Reps) {
		return training.SetInput{}, shared.Invalid("Reps must be 1–100")
	}
	in := training.SetInput{Weight: b.Weight, Reps: int(b.Reps)}
	if b.RIR != nil {
		if !whole(*b.RIR) {
			return training.SetInput{}, shared.Invalid("RIR must be 0–10")
		}
		rir := int(*b.RIR)
		in.RIR = &rir
	}
	return in, nil
}
