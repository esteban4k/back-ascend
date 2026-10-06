package httpapi

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"runtime/debug"
	"slices"
	"strings"
	"time"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// logRequests logs one line per request.
func logRequests(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		logger.Info("request", "method", r.Method, "path", r.URL.Path, "status", rec.status, "duration", time.Since(start).Round(time.Microsecond))
	})
}

// recoverPanics turns a panic into a 500 instead of dropping the connection.
func recoverPanics(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				if p == http.ErrAbortHandler {
					panic(p)
				}
				logger.Error("panic", "method", r.Method, "path", r.URL.Path, "panic", p, "stack", string(debug.Stack()))
				writeError(w, http.StatusInternalServerError, "internal", "Something went wrong")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// cors allows the configured origins to call the API from a browser.
// "*" allows any origin.
func cors(origins []string, next http.Handler) http.Handler {
	anyOrigin := slices.Contains(origins, "*")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowed := origin != "" && (anyOrigin || slices.Contains(origins, origin))
		if allowed {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Add("Vary", "Origin")
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			h.Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			if !allowed {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireToken checks `Authorization: Bearer <token>` when a token is configured.
// The health check stays public so orchestrators can probe it.
func requireToken(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	expected := []byte(token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/health" {
			next.ServeHTTP(w, r)
			return
		}
		got, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !found || subtle.ConstantTimeCompare([]byte(got), expected) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="ascend"`)
			writeError(w, http.StatusUnauthorized, "unauthorized", "Missing or invalid token")
			return
		}
		next.ServeHTTP(w, r)
	})
}
