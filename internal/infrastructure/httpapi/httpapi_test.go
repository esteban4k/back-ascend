package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ascend/internal/infrastructure/container"
	"ascend/internal/infrastructure/httpapi"
	"ascend/internal/infrastructure/seed"
)

var now = time.Date(2026, 9, 24, 8, 0, 0, 0, time.FixedZone("COT", -5*3600))

type client struct {
	t      *testing.T
	server *httptest.Server
	token  string
}

func newClient(t *testing.T, opts httpapi.Options) *client {
	t.Helper()
	uc, _ := container.ForTest(seed.Demo(now), now)
	if opts.Reset == nil {
		opts.Reset = func(context.Context) error { return nil }
	}
	opts.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(httpapi.NewHandler(uc, opts))
	t.Cleanup(server.Close)
	return &client{t: t, server: server}
}

func (c *client) do(method, path string, body any, headers ...string) (int, []byte, http.Header) {
	c.t.Helper()
	var reader io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		reader = strings.NewReader(b)
	default:
		raw, _ := json.Marshal(b)
		reader = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, c.server.URL+path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	return res.StatusCode, raw, res.Header
}

func (c *client) json(method, path string, body any, wantStatus int, out any) {
	c.t.Helper()
	status, raw, _ := c.do(method, path, body)
	if status != wantStatus {
		c.t.Fatalf("%s %s = %d, want %d: %s", method, path, status, wantStatus, raw)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			c.t.Fatalf("%s %s: decode %s: %v", method, path, raw, err)
		}
	}
}

func TestHabitCompletionOverHTTP(t *testing.T) {
	c := newClient(t, httpapi.Options{})
	var before, after struct {
		User struct {
			TotalXP int `json:"totalXp"`
		} `json:"user"`
	}
	c.json("GET", "/api/profile", nil, 200, &before)

	var reward struct {
		XP     int  `json:"xp"`
		Streak *int `json:"streak"`
		Level  struct {
			Level int `json:"level"`
		} `json:"level"`
	}
	c.json("POST", "/api/habits/read/complete", nil, 200, &reward)
	if reward.XP != 40 || reward.Streak == nil || reward.Level.Level != 17 {
		t.Fatalf("reward = %+v", reward)
	}
	c.json("GET", "/api/profile", nil, 200, &after)
	if after.User.TotalXP != before.User.TotalXP+40 {
		t.Fatalf("XP %d → %d", before.User.TotalXP, after.User.TotalXP)
	}

	var body struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	c.json("POST", "/api/habits/read/complete", nil, 422, &body)
	if body.Code != "validation" || body.Error != "Read 20 min is already complete" {
		t.Fatalf("error body = %+v", body)
	}
	c.json("POST", "/api/habits/nope/complete", nil, 404, &body)
	if body.Code != "not_found" {
		t.Fatalf("error body = %+v", body)
	}
}

func TestCreateHabitValidatesInput(t *testing.T) {
	c := newClient(t, httpapi.Options{})
	c.json("POST", "/api/habits", `{"name":`, 400, nil)
	c.json("POST", "/api/habits", nil, 400, nil)
	c.json("POST", "/api/habits", map[string]any{"name": "", "domainId": "physical", "xp": 20, "schedule": map[string]any{"kind": "daily"}}, 422, nil)

	var created struct {
		ID          string   `json:"id"`
		Completions []string `json:"completions"`
		Schedule    struct {
			Kind string `json:"kind"`
			Days []int  `json:"days"`
		} `json:"schedule"`
	}
	c.json("POST", "/api/habits", map[string]any{
		"name": "Stretch", "domainId": "physical", "xp": 20, "time": "07:30",
		"schedule": map[string]any{"kind": "weekly", "days": []int{2, 0, 2}},
	}, 201, &created)
	if created.ID == "" || created.Completions == nil || created.Schedule.Kind != "weekly" || len(created.Schedule.Days) != 2 {
		t.Fatalf("created = %+v", created)
	}
	var summary struct {
		Habit struct {
			Name string `json:"name"`
		} `json:"habit"`
		LastSeven []any `json:"lastSeven"`
	}
	c.json("GET", "/api/habits/"+created.ID, nil, 200, &summary)
	if summary.Habit.Name != "Stretch" || len(summary.LastSeven) != 7 {
		t.Fatalf("summary = %+v", summary)
	}
	status, _, _ := c.do("DELETE", "/api/habits/"+created.ID, nil)
	if status != 204 {
		t.Fatalf("delete = %d", status)
	}
}

func TestListsAreNeverNull(t *testing.T) {
	c := newClient(t, httpapi.Options{})
	for _, path := range []string{
		"/api/habits", "/api/objectives", "/api/domains", "/api/domains/options", "/api/journal",
		"/api/activity?limit=3", "/api/calendar?from=2026-09-21&to=2026-09-27", "/api/tasks?from=2030-01-01&to=2030-01-02",
		"/api/habits/consistency?days=14",
	} {
		status, raw, _ := c.do("GET", path, nil)
		if status != 200 || !bytes.HasPrefix(bytes.TrimSpace(raw), []byte("[")) {
			t.Errorf("GET %s = %d %.80s", path, status, raw)
		}
	}
	// These views have no legitimately-null fields (unlike training's
	// `current` or performance's future `thisWeek`).
	for _, path := range []string{"/api/today", "/api/focus", "/api/journey", "/api/objectives/current", "/api/domains"} {
		status, raw, _ := c.do("GET", path, nil)
		if status != 200 || bytes.Contains(raw, []byte(":null")) {
			t.Errorf("GET %s = %d %.200s", path, status, raw)
		}
	}
	for _, path := range []string{"/api/training", "/api/performance"} {
		if status, raw, _ := c.do("GET", path, nil); status != 200 {
			t.Errorf("GET %s = %d %.200s", path, status, raw)
		}
	}
}

func TestQueryValidation(t *testing.T) {
	c := newClient(t, httpapi.Options{})
	c.json("GET", "/api/calendar?from=2026-09-27&to=2026-09-21", nil, 422, nil)
	c.json("GET", "/api/calendar?from=yesterday&to=2026-09-21", nil, 422, nil)
	c.json("GET", "/api/habits/consistency?days=abc", nil, 400, nil)
	c.json("GET", "/api/habits/consistency?days=100000", nil, 422, nil)
	c.json("GET", "/api/nope", nil, 404, nil)
}

func TestFocusTimerRoundTrip(t *testing.T) {
	c := newClient(t, httpapi.Options{})
	var timer map[string]any
	c.json("POST", "/api/focus/timer", map[string]any{"label": "Deep work", "domainId": "career", "plannedMinutes": 50}, 200, &timer)
	if timer["pausedAt"] != nil || timer["label"] != "Deep work" {
		t.Fatalf("timer = %+v", timer)
	}
	timer["startedAt"] = now.Add(-50 * time.Minute).UnixMilli()
	var done struct {
		Session struct {
			Status string `json:"status"`
		} `json:"session"`
		Reward *struct {
			XP int `json:"xp"`
		} `json:"reward"`
	}
	c.json("POST", "/api/focus/sessions", timer, 201, &done)
	if done.Session.Status != "completed" || done.Reward == nil || done.Reward.XP != 83 {
		t.Fatalf("done = %+v", done)
	}
	c.json("POST", "/api/focus/timer", map[string]any{"label": "x", "domainId": "ghost", "plannedMinutes": 25}, 422, nil)
}

func TestWorkoutSetsAcceptClientNumbers(t *testing.T) {
	c := newClient(t, httpapi.Options{})
	status, _, _ := c.do("POST", "/api/habits/gym/uncomplete", nil)
	if status != 200 {
		t.Fatalf("uncomplete gym = %d", status)
	}
	var w struct {
		ID        string `json:"id"`
		Exercises []struct {
			ID string `json:"id"`
		} `json:"exercises"`
	}
	c.json("POST", "/api/workouts", map[string]any{"template": "Upper A"}, 201, &w)
	path := "/api/workouts/" + w.ID + "/exercises/" + w.Exercises[0].ID + "/sets"
	c.json("POST", path, map[string]any{"weight": 62.5, "reps": 8.5, "rir": nil}, 422, nil)
	var logged struct {
		IsRecord bool `json:"isRecord"`
		Set      struct {
			ID     string   `json:"id"`
			Weight *float64 `json:"weight"`
			RIR    *int     `json:"rir"`
		} `json:"set"`
	}
	c.json("POST", path, map[string]any{"weight": nil, "reps": 8, "rir": 2}, 201, &logged)
	if logged.Set.Weight != nil || logged.Set.RIR == nil || *logged.Set.RIR != 2 {
		t.Fatalf("logged = %+v", logged)
	}
	var reward struct {
		HabitName string `json:"habitName"`
		XP        int    `json:"xp"`
	}
	c.json("POST", "/api/workouts/"+w.ID+"/finish", map[string]any{"durationMinutes": 45}, 200, &reward)
	if reward.HabitName != "Gym" || reward.XP != 80 {
		t.Fatalf("reward = %+v", reward)
	}
}

func TestCORS(t *testing.T) {
	c := newClient(t, httpapi.Options{AllowedOrigins: []string{"http://localhost:5173"}})
	status, _, h := c.do("OPTIONS", "/api/habits", nil, "Origin", "http://localhost:5173", "Access-Control-Request-Method", "POST")
	if status != 204 || h.Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Fatalf("preflight = %d %v", status, h)
	}
	status, _, _ = c.do("OPTIONS", "/api/habits", nil, "Origin", "http://evil.test", "Access-Control-Request-Method", "POST")
	if status != 403 {
		t.Fatalf("foreign preflight = %d", status)
	}
	_, _, h = c.do("GET", "/api/today", nil, "Origin", "http://localhost:5173")
	if h.Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Fatal("simple request lacks CORS header")
	}
}

func TestTokenAuth(t *testing.T) {
	c := newClient(t, httpapi.Options{APIToken: "s3cret"})
	c.json("GET", "/api/health", nil, 200, nil)
	c.json("GET", "/api/today", nil, 401, nil)
	c.token = "wrong"
	c.json("GET", "/api/today", nil, 401, nil)
	c.token = "s3cret"
	c.json("GET", "/api/today", nil, 200, nil)
}

func TestResetCanBeDisabled(t *testing.T) {
	enabled := newClient(t, httpapi.Options{})
	if status, _, _ := enabled.do("POST", "/api/demo/reset", nil); status != 204 {
		t.Fatalf("reset = %d", status)
	}
	uc, _ := container.ForTest(seed.Demo(now), now)
	server := httptest.NewServer(httpapi.NewHandler(uc, httpapi.Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}))
	defer server.Close()
	res, err := http.Post(server.URL+"/api/demo/reset", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 404 {
		t.Fatalf("disabled reset = %d", res.StatusCode)
	}
}
