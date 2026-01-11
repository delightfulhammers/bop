package dedup

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSimpleClient is a test double for simple.Client.
type mockSimpleClient struct {
	response string
	err      error
	called   bool
	prompt   string
}

func (m *mockSimpleClient) Call(ctx context.Context, prompt string, maxTokens int) (string, error) {
	m.called = true
	m.prompt = prompt
	return m.response, m.err
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
