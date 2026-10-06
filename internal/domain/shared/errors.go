package shared

import (
	"errors"
	"fmt"
)

// ValidationError is a broken business rule. Its message is written for the
// person using the app, so adapters can show it as-is.
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

// Invalid builds a ValidationError.
func Invalid(message string) error {
	return &ValidationError{Message: message}
}

// Invalidf builds a ValidationError from a format string.
func Invalidf(format string, args ...any) error {
	return &ValidationError{Message: fmt.Sprintf(format, args...)}
}

// NotFoundError reports a missing entity.
type NotFoundError struct {
	Entity string
	ID     string
}

func (e *NotFoundError) Error() string { return fmt.Sprintf("%s %s not found", e.Entity, e.ID) }

// NotFound builds a NotFoundError.
func NotFound(entity, id string) error {
	return &NotFoundError{Entity: entity, ID: id}
}

// IsValidation reports whether err is (or wraps) a ValidationError.
func IsValidation(err error) bool {
	var target *ValidationError
	return errors.As(err, &target)
}

// IsNotFound reports whether err is (or wraps) a NotFoundError.
func IsNotFound(err error) bool {
	var target *NotFoundError
	return errors.As(err, &target)
}
