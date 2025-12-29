package http

import (
	"context"
	stderrors "errors"
	"fmt"
	"net"
)

// ErrorType represents the category of error that occurred.
type ErrorType int

const (
	ErrTypeAuthentication ErrorType = iota
	ErrTypeRateLimit
	ErrTypeServiceUnavailable
	ErrTypeInvalidRequest
	ErrTypeTimeout
	ErrTypeNetwork // General network errors (DNS, connection refused, TLS, etc.)
	ErrTypeModelNotFound
	ErrTypeContentFiltered
	ErrTypeUnknown
)

// String returns a human-readable description of the error type.
func (e ErrorType) String() string {
	switch e {
	case ErrTypeAuthentication:
		return "authentication error"
	case ErrTypeRateLimit:
		return "rate limit exceeded"
	case ErrTypeServiceUnavailable:
		return "service unavailable"
	case ErrTypeInvalidRequest:
		return "invalid request"
	case ErrTypeTimeout:
		return "timeout"
	case ErrTypeNetwork:
		return "network error"
	case ErrTypeModelNotFound:
		return "model not found"
	case ErrTypeContentFiltered:
		return "content filtered"
	case ErrTypeUnknown:
		return "unknown error"
	default:
		return "unknown error"
	}
}

// Error represents an HTTP client error with additional context.
type Error struct {
	Type       ErrorType
	Message    string
	StatusCode int
	Retryable  bool
	Provider   string
}

// Error implements the error interface.
func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s: %s (status: %d)", e.Provider, e.Type.String(), e.Message, e.StatusCode)
}

// Is implements error equality checking for errors.Is.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	return e.Type == t.Type
}

// IsRetryable returns true if the error is retryable.
func (e *Error) IsRetryable() bool {
	return e.Retryable
}

// NewAuthenticationError creates a new authentication error.
func NewAuthenticationError(provider, message string) *Error {
	return &Error{
		Type:       ErrTypeAuthentication,
		Message:    message,
		StatusCode: 401,
		Retryable:  false,
		Provider:   provider,
	}
}

// NewRateLimitError creates a new rate limit error.
func NewRateLimitError(provider, message string) *Error {
	return &Error{
		Type:       ErrTypeRateLimit,
		Message:    message,
		StatusCode: 429,
		Retryable:  true,
		Provider:   provider,
	}
}

// NewServiceUnavailableError creates a new service unavailable error.
func NewServiceUnavailableError(provider, message string) *Error {
	return &Error{
		Type:       ErrTypeServiceUnavailable,
		Message:    message,
		StatusCode: 503,
		Retryable:  true,
		Provider:   provider,
	}
}

// NewInvalidRequestError creates a new invalid request error.
func NewInvalidRequestError(provider, message string) *Error {
	return &Error{
		Type:       ErrTypeInvalidRequest,
		Message:    message,
		StatusCode: 400,
		Retryable:  false,
		Provider:   provider,
	}
}

// NewTimeoutError creates a new timeout error.
func NewTimeoutError(provider, message string) *Error {
	return &Error{
		Type:       ErrTypeTimeout,
		Message:    message,
		StatusCode: 0,
		Retryable:  true,
		Provider:   provider,
	}
}

// NewModelNotFoundError creates a new model not found error.
func NewModelNotFoundError(provider, message string) *Error {
	return &Error{
		Type:       ErrTypeModelNotFound,
		Message:    message,
		StatusCode: 404,
		Retryable:  false,
		Provider:   provider,
	}
}

// NewContentFilteredError creates a new content filtered error.
func NewContentFilteredError(provider, message string) *Error {
	return &Error{
		Type:       ErrTypeContentFiltered,
		Message:    message,
		StatusCode: 400,
		Retryable:  false,
		Provider:   provider,
	}
}

// NewNetworkError creates a new network error for general connectivity issues.
// Network errors are marked as retryable since they're typically transient.
func NewNetworkError(provider, message string) *Error {
	return &Error{
		Type:       ErrTypeNetwork,
		Message:    message,
		StatusCode: 0,
		Retryable:  true,
		Provider:   provider,
	}
}

// ClassifyNetworkError examines an error from http.Client.Do() and returns
// the appropriate typed error. It distinguishes between:
//   - Timeouts (context deadline exceeded, net.Error timeout)
//   - General network errors (DNS, connection refused, TLS, etc.)
//
// Both types are marked as retryable, but proper classification aids debugging.
func ClassifyNetworkError(provider string, err error, ctx context.Context) *Error {
	// Check for context deadline exceeded (timeout)
	if ctx.Err() == context.DeadlineExceeded {
		return NewTimeoutError(provider, "request timed out")
	}

	// Check for context cancellation (not a timeout, not retryable)
	if ctx.Err() == context.Canceled {
		return &Error{
			Type:       ErrTypeUnknown,
			Message:    "request canceled",
			StatusCode: 0,
			Retryable:  false,
			Provider:   provider,
		}
	}

	// Check if the underlying error is a timeout via net.Error interface
	var netErr net.Error
	if stderrors.As(err, &netErr) && netErr.Timeout() {
		return NewTimeoutError(provider, err.Error())
	}

	// Default to network error for other connectivity issues
	// (DNS failures, connection refused, TLS errors, etc.)
	return NewNetworkError(provider, err.Error())
}
