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
