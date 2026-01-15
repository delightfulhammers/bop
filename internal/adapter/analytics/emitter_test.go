package analytics

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platformanalytics "github.com/delightfulhammers/platform/contracts/analytics"
)

// mockPlatformEmitter captures emitted events for testing.
type mockPlatformEmitter struct {
	mu     sync.Mutex
	events []*platformanalytics.UsageEvent
}

func (m *mockPlatformEmitter) EmitAsync(_ context.Context, event *platformanalytics.UsageEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
}

func (m *mockPlatformEmitter) Events() []*platformanalytics.UsageEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.events
}

func TestBopEmitter_EmitReviewStarted(t *testing.T) {
	mock := &mockPlatformEmitter{}
	emitter := NewEmitter(mock, WithClientVersion("1.2.3"))

	tenantID := uuid.New()
	userID := uuid.New()
	data := ReviewEventData{
		TenantID:   tenantID,
		UserID:     &userID,
		SessionID:  "sess-123",
		ClientType: platformanalytics.ClientTypeCLI,
		Reviewers:  []string{"security", "architecture"},
		Provider:   "anthropic",
		Repository: "org/repo",
	}

	emitter.EmitReviewStarted(context.Background(), data)

	events := mock.Events()
	require.Len(t, events, 1)

	event := events[0]
	assert.Equal(t, ProductID, event.ProductID)
	assert.Equal(t, tenantID, event.TenantID)
	assert.Equal(t, &userID, event.UserID)
	assert.Equal(t, FeatureCodeReview, event.FeatureName)
	assert.Equal(t, platformanalytics.ClientTypeCLI, event.ClientType)
	assert.Equal(t, "1.2.3", event.ClientVersion)
	assert.Equal(t, "started", event.Properties["state"])
	assert.Equal(t, "anthropic", event.Properties["provider"])
	assert.Equal(t, "org/repo", event.Properties["repository"])
	assert.Equal(t, "security,architecture", event.Properties["reviewers"])
}

func TestBopEmitter_EmitReviewCompleted(t *testing.T) {
	mock := &mockPlatformEmitter{}
	emitter := NewEmitter(mock)

	tenantID := uuid.New()
	data := ReviewEventData{
		TenantID:   tenantID,
		ClientType: platformanalytics.ClientTypeMCP,
	}
	result := ReviewResult{
		DiffLines:     150,
		FilesReviewed: 12,
		FindingsCount: 3,
		DurationMs:    45000,
		PostedToGH:    true,
	}

	emitter.EmitReviewCompleted(context.Background(), data, result)

	events := mock.Events()
	require.Len(t, events, 1)

	event := events[0]
	assert.Equal(t, FeatureCodeReview, event.FeatureName)
	assert.Equal(t, "completed", event.Properties["state"])
	assert.True(t, event.Success)
	assert.Equal(t, int64(150), event.Metrics["diff_lines"])
	assert.Equal(t, int64(12), event.Metrics["files_reviewed"])
	assert.Equal(t, int64(3), event.Metrics["findings_count"])
	assert.Equal(t, 45000, event.DurationMs) // Duration is stored in DurationMs field, not Metrics
	assert.Equal(t, "true", event.Properties["posted_to_gh"])
}

func TestBopEmitter_EmitReviewFailed(t *testing.T) {
	mock := &mockPlatformEmitter{}
	emitter := NewEmitter(mock)

	tenantID := uuid.New()
	data := ReviewEventData{
		TenantID:   tenantID,
		ClientType: platformanalytics.ClientTypeAction,
	}

	emitter.EmitReviewFailed(context.Background(), data, "llm_timeout")

	events := mock.Events()
	require.Len(t, events, 1)

	event := events[0]
	assert.Equal(t, FeatureCodeReview, event.FeatureName)
	assert.Equal(t, "failed", event.Properties["state"])
	assert.False(t, event.Success)
	assert.Equal(t, "llm_timeout", event.ErrorCode)
}

func TestBopEmitter_EmitFindingsPosted(t *testing.T) {
	mock := &mockPlatformEmitter{}
	emitter := NewEmitter(mock)

	tenantID := uuid.New()
	data := ReviewEventData{
		TenantID:   tenantID,
		ClientType: platformanalytics.ClientTypeCLI,
	}

	emitter.EmitFindingsPosted(context.Background(), data, 5)

	events := mock.Events()
	require.Len(t, events, 1)

	event := events[0]
	assert.Equal(t, FeatureGitHubPost, event.FeatureName)
	assert.True(t, event.Success)
	assert.Equal(t, int64(5), event.Metrics["findings_count"])
}

func TestNopEmitter_DoesNothing(t *testing.T) {
	emitter := NopEmitter{}
	ctx := context.Background()
	data := ReviewEventData{}

	// These should not panic and should not produce any output
	emitter.EmitReviewStarted(ctx, data)
	emitter.EmitReviewCompleted(ctx, data, ReviewResult{})
	emitter.EmitReviewFailed(ctx, data, "error")
	emitter.EmitFindingsPosted(ctx, data, 10)
}

func TestBufferedEmitter_CapturesEvents(t *testing.T) {
	emitter := NewBufferedEmitter()
	ctx := context.Background()

	tenantID := uuid.New()
	data := ReviewEventData{
		TenantID:   tenantID,
		ClientType: platformanalytics.ClientTypeCLI,
	}

	// Emit various events
	emitter.EmitReviewStarted(ctx, data)
	emitter.EmitReviewCompleted(ctx, data, ReviewResult{FindingsCount: 3})
	emitter.EmitReviewFailed(ctx, data, "timeout")
	emitter.EmitFindingsPosted(ctx, data, 2)

	assert.Equal(t, 4, emitter.Count())

	// Verify event types
	assert.Equal(t, "review_started", emitter.Events[0].Type)
	assert.Equal(t, "review_completed", emitter.Events[1].Type)
	assert.Equal(t, "review_failed", emitter.Events[2].Type)
	assert.Equal(t, "findings_posted", emitter.Events[3].Type)

	// Verify review_completed result
	assert.NotNil(t, emitter.Events[1].Result)
	assert.Equal(t, 3, emitter.Events[1].Result.FindingsCount)

	// Verify review_failed error code
	assert.Equal(t, "timeout", emitter.Events[2].ErrorCode)

	// Test clear
	emitter.Clear()
	assert.Equal(t, 0, emitter.Count())
}

func TestJoinReviewers(t *testing.T) {
	tests := []struct {
		name      string
		reviewers []string
		expected  string
	}{
		{"empty", []string{}, ""},
		{"single", []string{"security"}, "security"},
		{"multiple", []string{"security", "architecture", "performance"}, "security,architecture,performance"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := joinReviewers(tt.reviewers)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBopEmitter_OptionalFields(t *testing.T) {
	mock := &mockPlatformEmitter{}
	emitter := NewEmitter(mock)

	// Emit with minimal data (no optional fields)
	tenantID := uuid.New()
	data := ReviewEventData{
		TenantID:   tenantID,
		ClientType: platformanalytics.ClientTypeCLI,
		// No UserID, SessionID, Reviewers, Provider, Repository
	}

	emitter.EmitReviewStarted(context.Background(), data)

	events := mock.Events()
	require.Len(t, events, 1)

	event := events[0]
	assert.Nil(t, event.UserID)
	assert.Empty(t, event.SessionID)
	assert.NotContains(t, event.Properties, "provider")
	assert.NotContains(t, event.Properties, "repository")
	assert.NotContains(t, event.Properties, "reviewers")
}
