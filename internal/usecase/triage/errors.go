package triage

import "errors"

// Port-level errors for PR-based triage operations.
// These are returned by port implementations when resources are not found.

// ErrAnnotationNotFound is returned when an annotation index is out of range.
var ErrAnnotationNotFound = errors.New("annotation not found")

// ErrCommentNotFound is returned when a PR comment doesn't exist.
var ErrCommentNotFound = errors.New("comment not found")

// ErrFileNotFound is returned when a file doesn't exist at the specified ref.
var ErrFileNotFound = errors.New("file not found")

// ErrCheckRunNotFound is returned when a check run ID doesn't exist.
var ErrCheckRunNotFound = errors.New("check run not found")

// ErrPRNotFound is returned when a pull request doesn't exist.
var ErrPRNotFound = errors.New("pull request not found")

// ErrNoCheckRuns is returned when no check runs match the filter criteria.
var ErrNoCheckRuns = errors.New("no matching check runs found")

// ErrNoAnnotations is returned when a check run has no annotations.
var ErrNoAnnotations = errors.New("check run has no annotations")

// ErrInvalidLineRange is returned when line numbers are invalid.
var ErrInvalidLineRange = errors.New("invalid line range")

// ErrNoSuggestion is returned when no code suggestion could be extracted.
var ErrNoSuggestion = errors.New("no suggestion found in content")

// ErrFileTruncated is returned when a file exceeds the maximum read size.
var ErrFileTruncated = errors.New("file truncated: exceeds 10MB limit")

// ErrInvalidFilter is returned when a filter value (severity, category, etc.) is invalid.
var ErrInvalidFilter = errors.New("invalid filter value")

// ValidSeverities contains all valid severity filter values.
// Used by ListFindings to validate the severity parameter.
var ValidSeverities = []string{"critical", "high", "medium", "low"}

// Write operation errors.

// ErrThreadNotFound is returned when a review thread doesn't exist.
var ErrThreadNotFound = errors.New("thread not found")

// ErrThreadAlreadyResolved is returned when trying to resolve an already-resolved thread.
// This is not an error condition for the handler - it should be treated as a no-op.
var ErrThreadAlreadyResolved = errors.New("thread already resolved")

// ErrThreadAlreadyUnresolved is returned when trying to unresolve an already-unresolved thread.
// This is not an error condition for the handler - it should be treated as a no-op.
var ErrThreadAlreadyUnresolved = errors.New("thread already unresolved")

// ErrLineOutOfBounds is returned when requested line numbers exceed the file length.
var ErrLineOutOfBounds = errors.New("line numbers out of bounds")

// ErrReviewNotFound is returned when a PR review doesn't exist.
var ErrReviewNotFound = errors.New("review not found")

// ErrUserNotFound is returned when a requested reviewer user doesn't exist.
var ErrUserNotFound = errors.New("user not found")

// ErrNotImplemented is returned when an operation is not supported by the adapter.
var ErrNotImplemented = errors.New("operation not implemented")

// ErrPermissionDenied is returned when the operation requires permissions not available.
var ErrPermissionDenied = errors.New("permission denied")
