package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"ascend/internal/domain/shared"
)

// maxBodyBytes bounds request bodies.
const maxBodyBytes = 1 << 20

// errorBody is the shape of every error response.
type errorBody struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

// badRequest is a malformed request (as opposed to a broken business rule).
type badRequest struct{ msg string }

func (e *badRequest) Error() string { return e.msg }

func badRequestf(msg string) error { return &badRequest{msg: msg} }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("encode response", "err", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, errorBody{Error: msg, Code: code})
}

// fail maps application errors to HTTP statuses.
func fail(w http.ResponseWriter, r *http.Request, err error) {
	var br *badRequest
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &br):
		writeError(w, http.StatusBadRequest, "bad_request", br.msg)
	case errors.As(err, &tooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "too_large", "Request body is too large")
	case shared.IsNotFound(err):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
	case shared.IsValidation(err):
		writeError(w, http.StatusUnprocessableEntity, "validation", err.Error())
	case errors.Is(err, context.Canceled):
		// The client went away; nobody is listening for the answer.
	default:
		slog.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "Something went wrong")
	}
}

// decode reads a JSON body into T.
func decode[T any](w http.ResponseWriter, r *http.Request) (T, error) {
	var v T
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&v); err != nil {
		var tooLarge *http.MaxBytesError
		switch {
		case errors.As(err, &tooLarge):
			return v, err
		case errors.Is(err, io.EOF):
			return v, badRequestf("Request body is required")
		default:
			return v, badRequestf("Invalid JSON: " + err.Error())
		}
	}
	return v, nil
}

// queryInt reads an optional integer query parameter.
func queryInt(r *http.Request, name string, fallback int) (int, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, badRequestf("Query parameter " + name + " must be an integer")
	}
	return v, nil
}

// endpoint is a handler that returns its result or an error.
type endpoint func(r *http.Request) (any, error)

// ok responds 200 with the endpoint's result.
func ok(fn endpoint) http.HandlerFunc { return respond(http.StatusOK, fn) }

// created responds 201 with the endpoint's result.
func created(fn endpoint) http.HandlerFunc { return respond(http.StatusCreated, fn) }

func respond(status int, fn endpoint) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v, err := fn(r)
		if err != nil {
			fail(w, r, err)
			return
		}
		writeJSON(w, status, v)
	}
}

// noContent responds 204 when the action succeeds.
func noContent(fn func(r *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := fn(r); err != nil {
			fail(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// withBody decodes the request body before calling fn.
func withBody[T any](fn func(r *http.Request, body T) (any, error)) func(w http.ResponseWriter, r *http.Request) (any, error) {
	return func(w http.ResponseWriter, r *http.Request) (any, error) {
		body, err := decode[T](w, r)
		if err != nil {
			return nil, err
		}
		return fn(r, body)
	}
}

// okBody responds 200 with the result of a handler that takes a JSON body.
func okBody[T any](fn func(r *http.Request, body T) (any, error)) http.HandlerFunc {
	return bodyStatus(http.StatusOK, fn)
}

// createdBody responds 201 with the result of a handler that takes a JSON body.
func createdBody[T any](fn func(r *http.Request, body T) (any, error)) http.HandlerFunc {
	return bodyStatus(http.StatusCreated, fn)
}

func bodyStatus[T any](status int, fn func(r *http.Request, body T) (any, error)) http.HandlerFunc {
	handle := withBody(fn)
	return func(w http.ResponseWriter, r *http.Request) {
		v, err := handle(w, r)
		if err != nil {
			fail(w, r, err)
			return
		}
		writeJSON(w, status, v)
	}
}
