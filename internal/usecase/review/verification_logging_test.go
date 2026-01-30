package review

import (
	"context"
	"sync"
	"testing"

	"github.com/delightfulhammers/bop/internal/domain"
)

// mockLogger captures log calls for testing
type mockLogger struct {
	mu         sync.Mutex
	debugCalls []mockLogCall
	infoCalls  []mockLogCall
	warnCalls  []mockLogCall
}

type mockLogCall struct {
	message string
	fields  map[string]interface{}
}

func (m *mockLogger) LogDebug(ctx context.Context, message string, fields map[string]interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.debugCalls = append(m.debugCalls, mockLogCall{message: message, fields: fields})
}

func (m *mockLogger) LogInfo(ctx context.Context, message string, fields map[string]interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.infoCalls = append(m.infoCalls, mockLogCall{message: message, fields: fields})
}

func (m *mockLogger) LogWarning(ctx context.Context, message string, fields map[string]interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.warnCalls = append(m.warnCalls, mockLogCall{message: message, fields: fields})
}

func TestLogVerificationDetails(t *testing.T) {
	logger := &mockLogger{}

	verified := []domain.VerifiedFinding{
		{
			Finding: domain.Finding{
				File:        "main.go",
				LineStart:   10,
				Severity:    "high",
				Description: "fmt not imported but file uses fmt.Println",
			},
			Verified:       false,
			Classification: "",
			Confidence:     95,
			Evidence:       "Import exists at line 4, this is a false positive",
		},
		{
			Finding: domain.Finding{
				File:        "main.go",
				LineStart:   20,
				Severity:    "high",
				Description: "nil pointer dereference possible",
			},
			Verified:        true,
			Classification:  domain.ClassBlockingBug,
			Confidence:      85,
			Evidence:        "No nil check before dereference at line 20",
			BlocksOperation: true,
		},
		{
			Finding: domain.Finding{
				File:        "util.go",
				LineStart:   5,
				Severity:    "medium",
				Description: "function is too long",
			},
			Verified:       true,
			Classification: domain.ClassStyle,
			Confidence:     50,
			Evidence:       "Style preference, not a real issue",
		},
	}

	// Only second finding is reportable
	reportable := []domain.VerifiedFinding{verified[1]}

	settings := VerificationSettings{
		ConfidenceDefault: 70,
		ConfidenceHigh:    60,
		ConfidenceMedium:  70,
	}

	logVerificationDetails(context.Background(), verified, reportable, settings, logger)

	// Verify summary log was emitted
	if len(logger.debugCalls) == 0 {
		t.Fatal("expected debug calls to be made")
	}

	// First call should be the summary
	summaryCall := logger.debugCalls[0]
	if summaryCall.message != "verification report" {
		t.Errorf("expected first call to be 'verification report', got %q", summaryCall.message)
	}
	if summaryCall.fields["total_findings"] != 3 {
		t.Errorf("expected total_findings=3, got %v", summaryCall.fields["total_findings"])
	}
	if summaryCall.fields["reportable"] != 1 {
		t.Errorf("expected reportable=1, got %v", summaryCall.fields["reportable"])
	}
	if summaryCall.fields["filtered"] != 2 {
		t.Errorf("expected filtered=2, got %v", summaryCall.fields["filtered"])
	}

	// Should have 3 finding detail logs + 1 summary = 4 total
	if len(logger.debugCalls) != 4 {
		t.Errorf("expected 4 debug calls (1 summary + 3 findings), got %d", len(logger.debugCalls))
	}

	// Check first finding (not verified - should be filtered)
	finding1 := logger.debugCalls[1]
	if finding1.fields["status"] != "FILTERED" {
		t.Errorf("expected first finding status=FILTERED, got %v", finding1.fields["status"])
	}
	if finding1.fields["filter_reason"] != "NOT_VERIFIED" {
		t.Errorf("expected first finding filter_reason=NOT_VERIFIED, got %v", finding1.fields["filter_reason"])
	}

	// Check second finding (verified and passes threshold - should pass)
	finding2 := logger.debugCalls[2]
	if finding2.fields["status"] != "PASS" {
		t.Errorf("expected second finding status=PASS, got %v", finding2.fields["status"])
	}

	// Check third finding (verified but below threshold - should be filtered)
	finding3 := logger.debugCalls[3]
	if finding3.fields["status"] != "FILTERED" {
		t.Errorf("expected third finding status=FILTERED, got %v", finding3.fields["status"])
	}
	filterReason, ok := finding3.fields["filter_reason"].(string)
	if !ok || filterReason == "" {
		t.Errorf("expected third finding to have CONFIDENCE_BELOW_THRESHOLD reason, got %v", finding3.fields["filter_reason"])
	}
}

func TestLogVerificationDetails_EmptyFindings(t *testing.T) {
	logger := &mockLogger{}

	logVerificationDetails(
		context.Background(),
		[]domain.VerifiedFinding{},
		[]domain.VerifiedFinding{},
		VerificationSettings{},
		logger,
	)

	// Should have exactly 1 call (summary only, no findings)
	if len(logger.debugCalls) != 1 {
		t.Errorf("expected 1 debug call (summary only), got %d", len(logger.debugCalls))
	}

	summaryCall := logger.debugCalls[0]
	if summaryCall.fields["total_findings"] != 0 {
		t.Errorf("expected total_findings=0, got %v", summaryCall.fields["total_findings"])
	}
	if summaryCall.fields["reportable"] != 0 {
		t.Errorf("expected reportable=0, got %v", summaryCall.fields["reportable"])
	}
}

func TestLogVerificationDetails_NilLogger(t *testing.T) {
	// When logger is nil, function should return early without panicking
	logVerificationDetails(
		context.Background(),
		[]domain.VerifiedFinding{{Finding: domain.Finding{File: "test.go"}}},
		[]domain.VerifiedFinding{},
		VerificationSettings{},
		nil,
	)
	// Success if no panic
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is longer than ten", 10, "this is lo"},
		{"", 5, ""},
		{"test", 0, ""},
	}

	for _, tt := range tests {
		result := truncateString(tt.input, tt.maxLen)
		if result != tt.expected {
			t.Errorf("truncateString(%q, %d) = %q, want %q",
				tt.input, tt.maxLen, result, tt.expected)
		}
	}
}
