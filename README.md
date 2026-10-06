# Ascend — API

REST backend for [Ascend](../front-ascend): habits, objectives, focus, reflection and training, all feeding one progression system (XP → level → journey).

Written in Go with the same hexagonal architecture as the web client. The business rules (streaks, XP curve, objective progress, focus timer, PRs) and use cases are ported one-to-one, so the server returns exactly what the client computes today with its mock adapters.

```bash
go run ./cmd/api          # http://localhost:8080/api
go test ./...             # domain, use case, SQLite, HTTP and architecture tests
go vet ./...
go build -o bin/ascend-api ./cmd/api
```

Requires Go 1.27+. No C compiler needed: SQLite uses a pure-Go driver.

On first start the database is empty, so it is seeded with the same demo account as the web client (~6 months of history relative to today). `POST /api/demo/reset` restores it.

---

## Architecture

Hexagonal (ports & adapters). Dependencies point inward:

```
infrastructure ──▶ application ──▶ domain
 (http, sqlite,      (use cases,     (entities and
  memory, seed)       ports)          pure rules)
```

`internal/architecture_test.go` enforces this: the suite fails if `domain/` or `application/` imports outward, or touches HTTP, SQL, the filesystem or third-party modules.

| Question | Where |
|---|---|
| Business rules | `internal/domain/` — pure Go. `habit`, `objective`, `focus`, `training`, `progression`, `lifedomain`, `journal`, `journey`, `task`, `activity`, `shared` (dates, errors, rounding). |
| Use cases | `internal/application/<feature>/` — `habits.Complete`, `objectives.RecordValue`, `focus.Complete`, `training.LogSet`… |
| Ports (interfaces) | `internal/application/ports/` — repositories, `Clock`, `IDGenerator`, `Transactor` |
| Adapters | `internal/infrastructure/` — `sqlite/` (default), `memory/` (tests, dev), `httpapi/` (REST), `system/` (clock, UUIDs), `seed/` (demo and blank data) |
| Wiring | `internal/infrastructure/container/` — the composition root, the only place that picks adapters |
| Entry point | `cmd/api/main.go` |

| Web client (`front-ascend/src`) | This repo |
|---|---|
| `domain/habit/Habit.ts` | `internal/domain/habit/habit.go` |
| `application/habits/habitUseCases.ts` | `internal/application/habits/habits.go` |
| `application/progression/rewards.ts` | `internal/application/progression/rewards.go` |
| `application/ports/` | `internal/application/ports/ports.go` |
| `infrastructure/mock/` | `internal/infrastructure/memory/` |
| `infrastructure/mock/seed/` | `internal/infrastructure/seed/` |
| `infrastructure/container.ts` | `internal/infrastructure/container/container.go` |
| `presentation/` | `internal/infrastructure/httpapi/` (the delivery adapter) |

### Domain

Entities are plain structs plus pure functions (`habit.Complete(h, date)`, `habit.CurrentStreak(h, today)`, `objective.Progress(o)`). They never mutate their input: every change returns a new value. Validation lives here too and returns `shared.ValidationError`, whose message is written for the person using the app.

Dates are `shared.LocalDate` (`YYYY-MM-DD`); day arithmetic is done in UTC so it has no DST gaps. Instants are `time.Time`, stored in UTC; they are turned into dates with the clock's location (the user's timezone).

### Application

Each feature is a service bound to `ports.Ports`:

```go
func (s *Service) Complete(ctx context.Context, id string) (progression.Reward, error) {
	return ports.InTx(ctx, s.p.Tx, func(ctx context.Context) (progression.Reward, error) {
		return complete(ctx, s.p, id)
	})
}
```

`progression.GrantReward` is the single path through which progress is registered: XP to the user, score to the life domain, an entry in the activity log and, on level-up, a journey milestone. `RevokeReward` undoes it exactly.

Every mutation runs in a unit of work (`ports.Transactor`): all of its writes commit together or not at all. Nested units join the outer one, so saving a journal entry that completes the Journal habit is still one transaction.

Some actions trigger others through the domain: saving a journal entry completes the habit with `trigger: "journal"`; finishing a workout completes the one with `trigger: "workout"`.

The server also checks references the client could not: drafts must point at an existing domain, habits and objective.

### Infrastructure

- **`sqlite/`** stores each entity as a JSON document in its own table, with the columns queries filter on (`tasks.date`, `activity.at`, `activity.ref_id`) promoted and indexed. Migrations are embedded and applied on start. One connection, WAL mode, `BEGIN IMMEDIATE` transactions.
- **`memory/`** keeps everything in process memory, copy-on-write, and rolls back failed units of work by restoring a snapshot. It backs the use case tests (`container.ForTest`) and `ASCEND_STORE=memory`.
- **`httpapi/`** maps routes to use cases and errors to status codes. No business rules.

To add another database (e.g. PostgreSQL), implement the ports in a new package and select it in `container.Build`. Nothing in `domain/` or `application/` changes.

---

## Configuration

Environment variables (see `.env.example`):

| Variable | Default | |
|---|---|---|
| `ASCEND_ADDR` | `:8080` | Listen address. `PORT` overrides it (`:$PORT`). |
| `ASCEND_STORE` | `sqlite` | `sqlite` or `memory` |
| `ASCEND_DB_PATH` | `data/ascend.db` | SQLite file (directories are created) |
| `ASCEND_TIMEZONE` | `Local` | IANA zone that decides what "today" is, e.g. `America/Bogota` |
| `ASCEND_SEED` | `demo` | Data for an empty store: `demo` (6 months of history) or `blank` (user, default domains, origin milestone) |
| `ASCEND_USER_NAME` | `You` | User name for the `blank` seed |
| `ASCEND_ALLOW_RESET` | `true` | Enables `POST /api/demo/reset` |
| `ASCEND_CORS_ORIGINS` | `http://localhost:5173,http://127.0.0.1:5173` | Comma-separated; `*` allows any origin |
| `ASCEND_API_TOKEN` | — | When set, every route except `/api/health` requires `Authorization: Bearer <token>` |
| `ASCEND_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |

---

## API

Base URL `/api`. JSON in and out, camelCase fields matching the client's TypeScript types. Dates are `YYYY-MM-DD`; instants are ISO 8601.

Errors have the shape `{ "error": "Give the habit a name", "code": "validation" }`:

| Status | `code` | When |
|---|---|---|
| 400 | `bad_request` | Malformed JSON or query parameter |
| 401 | `unauthorized` | Missing or wrong token (only when `ASCEND_API_TOKEN` is set) |
| 404 | `not_found` | Unknown entity or route |
| 413 | `too_large` | Body over 1 MB |
| 422 | `validation` | A business rule was broken; show `error` to the user |
| 500 | `internal` | Unexpected; details are in the server log |

### Routes

Each route maps to the client use case in the right column (`useCases.<group>.<name>`).

| Method & path | Body / query | Returns | Client use case |
|---|---|---|---|
| `GET /health` | | `{status, dataSource}` | |
| `POST /demo/reset` | | 204 | Settings → Reset demo data |
| `GET /profile` | | `{user, level}` | `profile.get` |
| `PATCH /profile` | `{name}` | `{user, level}` | `profile.rename` |
| `GET /activity` | `?limit=12` | `ActivityEvent[]` newest first | `profile.activity` |
| `GET /today` | | `TodayOverview` | `today.get` |
| `GET /tasks` | `?from&to` | `Task[]` | |
| `POST /tasks` | `TaskDraft` | 201 `Task` | `today.createTask` |
| `POST /tasks/{id}/complete` | | `Reward` | `today.completeTask` |
| `POST /tasks/{id}/reopen` | | `LevelState` | `today.reopenTask` |
| `DELETE /tasks/{id}` | | 204 | `today.deleteTask` |
| `GET /habits` | | `HabitSummary[]` | `habits.list` |
| `GET /habits/consistency` | `?days=140&habitId=` | `ConsistencyCell[]` | `habits.consistency` |
| `GET /habits/{id}` | | `HabitSummary` | |
| `POST /habits` | `HabitDraft` | 201 `Habit` | `habits.create` |
| `PUT /habits/{id}` | `HabitDraft` | `Habit` | `habits.update` |
| `DELETE /habits/{id}` | | 204 | `habits.delete` |
| `POST /habits/{id}/complete` | | `Reward` (with `streak`) | `habits.complete` |
| `POST /habits/{id}/uncomplete` | | `LevelState` | `habits.uncomplete` |
| `GET /objectives` | | `ObjectiveSummary[]` | `objectives.list` |
| `GET /objectives/current` | | `ObjectiveSummary \| null` | `objectives.current` |
| `GET /objectives/{id}` | | `ObjectiveSummary` | |
| `POST /objectives` | `ObjectiveDraft` | 201 `Objective` | `objectives.create` |
| `POST /objectives/{id}/value` | `{value}` | `Reward \| null` | `objectives.recordValue` |
| `POST /objectives/{id}/milestones/{milestoneId}/toggle` | | `Reward \| null` | `objectives.toggleMilestone` |
| `PUT /objectives/{id}/status` | `{status}` | `Objective` | `objectives.setStatus` |
| `DELETE /objectives/{id}` | | 204 | `objectives.delete` |
| `GET /domains` | | `DomainSummary[]` | `domains.list` |
| `GET /domains/options` | | `{id, name}[]` | `domains.options` |
| `POST /domains` | `{name, description}` | 201 `LifeDomain` | `domains.create` |
| `PUT /domains/{id}` | `{name, description}` | `LifeDomain` | `domains.update` |
| `DELETE /domains/{id}` | | 204 | `domains.delete` |
| `GET /journal` | | `JournalEntry[]` newest first | `journal.list` |
| `GET /journal/{id}` | | `JournalEntry` | |
| `POST /journal` | `JournalDraft` | 201 `{entry, reward}` | `journal.create` |
| `PUT /journal/{id}` | `JournalDraft` | `JournalEntry` | `journal.update` |
| `DELETE /journal/{id}` | | 204 | `journal.delete` |
| `GET /focus` | | `FocusOverview` | `focus.overview` |
| `POST /focus/timer` | `{label, domainId, plannedMinutes}` | `FocusTimer` | `focus.start` |
| `POST /focus/sessions` | `FocusTimer` | 201 `{session, reward}` | `focus.complete` |
| `GET /training` | | `TrainingOverview` | `training.overview` |
| `GET /workouts/{id}` | | `Workout` | |
| `POST /workouts` | `{template}` | 201 `Workout` | `training.start` |
| `DELETE /workouts/{id}` | | 204 | `training.discard` |
| `POST /workouts/{id}/exercises` | `{name}` | 201 `Workout` | `training.addExercise` |
| `POST /workouts/{id}/exercises/{exerciseId}/sets` | `{weight, reps, rir}` | 201 `{set, isRecord}` | `training.logSet` |
| `DELETE /workouts/{id}/exercises/{exerciseId}/sets/{setId}` | | `Workout` | `training.removeSet` |
| `POST /workouts/{id}/finish` | `{durationMinutes}` | `Reward` (+ `habitName` if a habit was completed) | `training.finish` |
| `GET /calendar` | `?from&to` (max 400 days) | `CalendarItem[]` | `calendar.agenda` |
| `GET /performance` | | `PerformanceReport` | `performance.report` |
| `GET /journey` | | `{user, level, milestones, stats}` | `journey.get` |

The focus timer runs on the client, as it does today: `POST /focus/timer` validates and returns a `FocusTimer` (epoch-ms timestamps); the client pauses and resumes it locally and sends it back to `POST /focus/sessions` when finished. The server validates it and caps focused time at the planned duration.

```bash
curl -X POST localhost:8080/api/habits/read/complete
# {"xp":40,"level":{"level":17,"xpInLevel":7460,"xpForNext":10000,"progress":0.746},"leveledUp":false,"streak":9}
```

---

## Connecting the web client

The client's use cases currently compute everything against local mock repositories. With this API the rules live on the server, so each use case becomes a single call. The cleanest switch is an HTTP implementation of the `UseCases` object in `front-ascend/src/infrastructure/http/`, chosen in `container.ts`, e.g.

```ts
habits: {
  list: () => http.get<HabitSummary[]>('/habits'),
  complete: (id: string) => http.post<Reward>(`/habits/${id}/complete`),
  …
}
```

and `VITE_API_URL=http://localhost:8080/api`. Presentation code does not change: the response shapes are the ones it already consumes.
