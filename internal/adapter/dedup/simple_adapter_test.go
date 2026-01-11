package dedup

import (
	"context"
	"errors"
	"testing"

	"github.com/delightfulhammers/bop/internal/adapter/llm/simple"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSimpleClient is a test double for simple.Client.
type mockSimpleClient struct {
	response string
	usage    simple.Usage
	err      error
	called   bool
	prompt   string
}

func (m *mockSimpleClient) Call(ctx context.Context, prompt string, maxTokens int) (string, simple.Usage, error) {
	m.called = true
	m.prompt = prompt
	return m.response, m.usage, m.err
}

func TestSimpleClientAdapter_Compare_Success(t *testing.T) {
	mock := &mockSimpleClient{
		response: `{"comparisons": [{"pair_index": 0, "is_duplicate": true, "reason": "Same issue"}]}`,
	}

	adapter := NewSimpleClientAdapter(mock)

	result, err := adapter.Compare(context.Background(), "test prompt", 4096)

	require.NoError(t, err)
	assert.True(t, mock.called)
	assert.Equal(t, "test prompt", mock.prompt)
	assert.Equal(t, mock.response, result)
}

func TestSimpleClientAdapter_Compare_Error(t *testing.T) {
	expectedErr := errors.New("API rate limit exceeded")
	mock := &mockSimpleClient{
		err: expectedErr,
	}

	adapter := NewSimpleClientAdapter(mock)

	result, err := adapter.Compare(context.Background(), "test prompt", 4096)

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
	assert.Empty(t, result)
	assert.True(t, mock.called)
}

func TestSimpleClientAdapter_Compare_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	mock := &mockSimpleClient{
		err: context.Canceled,
	}

	adapter := NewSimpleClientAdapter(mock)

	_, err := adapter.Compare(ctx, "test prompt", 4096)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestSimpleClientAdapter_TotalUsage_Accumulates(t *testing.T) {
	mock := &mockSimpleClient{
		response: "response",
		usage:    simple.Usage{InputTokens: 100, OutputTokens: 50},
	}

	adapter := NewSimpleClientAdapter(mock)

	// First call
	_, err := adapter.Compare(context.Background(), "prompt1", 4096)
	require.NoError(t, err)

	usage := adapter.TotalUsage()
	assert.Equal(t, 100, usage.InputTokens)
	assert.Equal(t, 50, usage.OutputTokens)

	// Second call - usage should accumulate
	_, err = adapter.Compare(context.Background(), "prompt2", 4096)
	require.NoError(t, err)

	usage = adapter.TotalUsage()
	assert.Equal(t, 200, usage.InputTokens)
	assert.Equal(t, 100, usage.OutputTokens)
}

func TestSimpleClientAdapter_ResetUsage(t *testing.T) {
	mock := &mockSimpleClient{
		response: "response",
		usage:    simple.Usage{InputTokens: 100, OutputTokens: 50},
	}

	adapter := NewSimpleClientAdapter(mock)

	// Accumulate some usage
	_, err := adapter.Compare(context.Background(), "prompt", 4096)
	require.NoError(t, err)
	assert.Equal(t, 100, adapter.TotalUsage().InputTokens)

	// Reset should clear usage
	adapter.ResetUsage()
	usage := adapter.TotalUsage()
	assert.Equal(t, 0, usage.InputTokens)
	assert.Equal(t, 0, usage.OutputTokens)
}

func TestSimpleClientAdapter_Error_DoesNotAccumulateUsage(t *testing.T) {
	mock := &mockSimpleClient{
		err:   errors.New("API error"),
		usage: simple.Usage{InputTokens: 100, OutputTokens: 50},
	}

	adapter := NewSimpleClientAdapter(mock)

	_, err := adapter.Compare(context.Background(), "prompt", 4096)
	require.Error(t, err)

	// Usage should not be accumulated on error
	usage := adapter.TotalUsage()
	assert.Equal(t, 0, usage.InputTokens)
	assert.Equal(t, 0, usage.OutputTokens)
}

func TestSimpleClientAdapter_ImplementsUsageProvider(t *testing.T) {
	mock := &mockSimpleClient{}
	adapter := NewSimpleClientAdapter(mock)

	// Verify the adapter implements UsageProvider
	var _ UsageProvider = adapter
}
