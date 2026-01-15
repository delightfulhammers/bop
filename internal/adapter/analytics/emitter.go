// Package analytics provides a wrapper around the platform analytics client
// for emitting bop-specific usage events.
package analytics

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	platformanalytics "github.com/delightfulhammers/platform/contracts/analytics"
	"github.com/delightfulhammers/platform/pkg/analytics"
)

// ProductID is the analytics product identifier for bop.
const ProductID = "bop"

// Feature names for analytics events.
const (
	FeatureCodeReview = "code_review"
	FeatureGitHubPost = "github_post"
)

// Emitter defines the interface for emitting bop analytics events.
// This interface allows for easy mocking in tests.
type Emitter interface {
	// EmitReviewStarted records the start of a code review.
	EmitReviewStarted(ctx context.Context, data ReviewEventData)

	// EmitReviewCompleted records successful completion of a code review.
	EmitReviewCompleted(ctx context.Context, data ReviewEventData, result ReviewResult)

	// EmitReviewFailed records a failed code review.
	EmitReviewFailed(ctx context.Context, data ReviewEventData, errCode string)

	// EmitFindingsPosted records posting findings to GitHub.
	EmitFindingsPosted(ctx context.Context, data ReviewEventData, findingsCount int)
}

// PlatformEmitter is the interface that platform analytics clients implement.
type PlatformEmitter interface {
	EmitAsync(ctx context.Context, event *platformanalytics.UsageEvent)
}

// ReviewEventData contains common fields for review-related events.
type ReviewEventData struct {
	TenantID      uuid.UUID
	UserID        *uuid.UUID
	SessionID     string
	ClientType    platformanalytics.ClientType
	ClientVersion string
	Reviewers     []string
	Provider      string
	Repository    string
}

// ReviewResult contains metrics from a completed review.
type ReviewResult struct {
	DiffLines     int
	FilesReviewed int
	FindingsCount int
	DurationMs    int
	PostedToGH    bool
}

// BopEmitter wraps the platform analytics client with bop-specific helpers.
type BopEmitter struct {
	platform      PlatformEmitter
	clientVersion string
	logger        *slog.Logger
}

// EmitterOption configures a BopEmitter.
type EmitterOption func(*BopEmitter)

// WithClientVersion sets the client version for emitted events.
func WithClientVersion(version string) EmitterOption {
	return func(e *BopEmitter) {
		e.clientVersion = version
	}
}

// WithLogger sets a custom logger.
func WithLogger(logger *slog.Logger) EmitterOption {
	return func(e *BopEmitter) {
		e.logger = logger
	}
}

// NewEmitter creates a new BopEmitter wrapping the platform analytics client.
func NewEmitter(platform PlatformEmitter, opts ...EmitterOption) *BopEmitter {
	e := &BopEmitter{
		platform: platform,
		logger:   slog.Default(),
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// EmitReviewStarted records the start of a code review.
func (e *BopEmitter) EmitReviewStarted(ctx context.Context, data ReviewEventData) {
	event, err := e.buildEvent(data, FeatureCodeReview).
		WithProperty("state", "started").
		Build()
	if err != nil {
		e.logger.Warn("failed to build review_started event", "error", err)
		return
	}
	e.platform.EmitAsync(ctx, event)
}

// EmitReviewCompleted records successful completion of a code review.
func (e *BopEmitter) EmitReviewCompleted(ctx context.Context, data ReviewEventData, result ReviewResult) {
	builder := e.buildEvent(data, FeatureCodeReview).
		WithProperty("state", "completed").
		WithDuration(result.DurationMs).
		WithMetric("diff_lines", int64(result.DiffLines)).
		WithMetric("files_reviewed", int64(result.FilesReviewed)).
		WithMetric("findings_count", int64(result.FindingsCount)).
		Success()

	if result.PostedToGH {
		builder = builder.WithProperty("posted_to_gh", "true")
	}

	event, err := builder.Build()
	if err != nil {
		e.logger.Warn("failed to build review_completed event", "error", err)
		return
	}
	e.platform.EmitAsync(ctx, event)
}

// EmitReviewFailed records a failed code review.
func (e *BopEmitter) EmitReviewFailed(ctx context.Context, data ReviewEventData, errCode string) {
	event, err := e.buildEvent(data, FeatureCodeReview).
		WithProperty("state", "failed").
		Failure(errCode).
		Build()
	if err != nil {
		e.logger.Warn("failed to build review_failed event", "error", err)
		return
	}
	e.platform.EmitAsync(ctx, event)
}

// EmitFindingsPosted records posting findings to GitHub.
func (e *BopEmitter) EmitFindingsPosted(ctx context.Context, data ReviewEventData, findingsCount int) {
	event, err := e.buildEvent(data, FeatureGitHubPost).
		WithMetric("findings_count", int64(findingsCount)).
		Success().
		Build()
	if err != nil {
		e.logger.Warn("failed to build findings_posted event", "error", err)
		return
	}
	e.platform.EmitAsync(ctx, event)
}

// buildEvent creates an EventBuilder with common fields populated.
func (e *BopEmitter) buildEvent(data ReviewEventData, featureName string) *analytics.EventBuilder {
	builder := analytics.NewEvent(ProductID, data.TenantID, featureName, data.ClientType)

	if data.UserID != nil {
		builder = builder.WithUser(*data.UserID)
	}
	if data.SessionID != "" {
		builder = builder.WithSession(data.SessionID)
	}

	// Use configured client version, fallback to data's version
	version := e.clientVersion
	if version == "" {
		version = data.ClientVersion
	}
	if version != "" {
		builder = builder.WithClientVersion(version)
	}

	// Add optional properties
	if data.Provider != "" {
		builder = builder.WithProperty("provider", data.Provider)
	}
	if data.Repository != "" {
		builder = builder.WithProperty("repository", data.Repository)
	}
	if len(data.Reviewers) > 0 {
		builder = builder.WithProperty("reviewers", joinReviewers(data.Reviewers))
	}

	return builder
}

// joinReviewers concatenates reviewer names for the properties map.
func joinReviewers(reviewers []string) string {
	return strings.Join(reviewers, ",")
}

// NopEmitter is an Emitter that does nothing.
// Used when analytics is disabled.
type NopEmitter struct{}

// EmitReviewStarted does nothing.
func (NopEmitter) EmitReviewStarted(_ context.Context, _ ReviewEventData) {}

// EmitReviewCompleted does nothing.
func (NopEmitter) EmitReviewCompleted(_ context.Context, _ ReviewEventData, _ ReviewResult) {}

// EmitReviewFailed does nothing.
func (NopEmitter) EmitReviewFailed(_ context.Context, _ ReviewEventData, _ string) {}

// EmitFindingsPosted does nothing.
func (NopEmitter) EmitFindingsPosted(_ context.Context, _ ReviewEventData, _ int) {}

// BufferedEmitter collects emitted events for testing.
type BufferedEmitter struct {
	Events []BufferedEvent
}

// BufferedEvent captures an emitted event for testing.
type BufferedEvent struct {
	Type      string
	Data      ReviewEventData
	Result    *ReviewResult
	ErrorCode string
	Timestamp time.Time
}

// NewBufferedEmitter creates a new buffered emitter for testing.
func NewBufferedEmitter() *BufferedEmitter {
	return &BufferedEmitter{
		Events: make([]BufferedEvent, 0),
	}
}

// EmitReviewStarted records a review_started event.
func (b *BufferedEmitter) EmitReviewStarted(_ context.Context, data ReviewEventData) {
	b.Events = append(b.Events, BufferedEvent{
		Type:      "review_started",
		Data:      data,
		Timestamp: time.Now(),
	})
}

// EmitReviewCompleted records a review_completed event.
func (b *BufferedEmitter) EmitReviewCompleted(_ context.Context, data ReviewEventData, result ReviewResult) {
	b.Events = append(b.Events, BufferedEvent{
		Type:      "review_completed",
		Data:      data,
		Result:    &result,
		Timestamp: time.Now(),
	})
}

// EmitReviewFailed records a review_failed event.
func (b *BufferedEmitter) EmitReviewFailed(_ context.Context, data ReviewEventData, errCode string) {
	b.Events = append(b.Events, BufferedEvent{
		Type:      "review_failed",
		Data:      data,
		ErrorCode: errCode,
		Timestamp: time.Now(),
	})
}

// EmitFindingsPosted records a findings_posted event.
func (b *BufferedEmitter) EmitFindingsPosted(_ context.Context, data ReviewEventData, findingsCount int) {
	result := ReviewResult{FindingsCount: findingsCount}
	b.Events = append(b.Events, BufferedEvent{
		Type:      "findings_posted",
		Data:      data,
		Result:    &result,
		Timestamp: time.Now(),
	})
}

// Clear removes all buffered events.
func (b *BufferedEmitter) Clear() {
	b.Events = b.Events[:0]
}

// Count returns the number of buffered events.
func (b *BufferedEmitter) Count() int {
	return len(b.Events)
}

// Compile-time interface checks.
var (
	_ Emitter = (*BopEmitter)(nil)
	_ Emitter = NopEmitter{}
	_ Emitter = (*BufferedEmitter)(nil)
)
