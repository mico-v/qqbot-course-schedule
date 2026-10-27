package schedule

import (
	"errors"
	"fmt"
)

// ErrorKind is the stable error category exposed by application boundaries.
type ErrorKind string

const (
	ErrorValidation     ErrorKind = "validation"
	ErrorUnauthorized   ErrorKind = "unauthorized"
	ErrorForbidden      ErrorKind = "forbidden"
	ErrorNotFound       ErrorKind = "not_found"
	ErrorConflict       ErrorKind = "conflict"
	ErrorGone           ErrorKind = "gone"
	ErrorTooManyRequest ErrorKind = "too_many_requests"
	ErrorInternal       ErrorKind = "internal"
)

// ClassifiedError adds a stable category while preserving the underlying
// cause for errors.Is/errors.As.
type ClassifiedError struct {
	Kind ErrorKind
	Err  error
}

func (e *ClassifiedError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *ClassifiedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// WithErrorKind wraps err unless it already has a category.
func WithErrorKind(kind ErrorKind, err error) error {
	if err == nil {
		return nil
	}
	var classified *ClassifiedError
	if errors.As(err, &classified) {
		return err
	}
	return &ClassifiedError{Kind: kind, Err: err}
}

// ValidationErrorf creates a validation error with a user-facing Chinese
// message.
func ValidationErrorf(format string, args ...any) error {
	return WithErrorKind(ErrorValidation, fmt.Errorf(format, args...))
}

// NotFoundErrorf creates a not-found error.
func NotFoundErrorf(format string, args ...any) error {
	return WithErrorKind(ErrorNotFound, fmt.Errorf(format, args...))
}

// ConflictErrorf creates a conflict error.
func ConflictErrorf(format string, args ...any) error {
	return WithErrorKind(ErrorConflict, fmt.Errorf(format, args...))
}

// ErrorKindOf returns the stable category for err.
func ErrorKindOf(err error) ErrorKind {
	if err == nil {
		return ""
	}
	var classified *ClassifiedError
	if errors.As(err, &classified) {
		return classified.Kind
	}
	switch {
	case errors.Is(err, ErrConflict):
		return ErrorConflict
	case errors.Is(err, ErrScheduleEditLinkExpired):
		return ErrorGone
	case errors.Is(err, ErrScheduleEditLinkInvalid):
		return ErrorNotFound
	default:
		return ErrorInternal
	}
}
