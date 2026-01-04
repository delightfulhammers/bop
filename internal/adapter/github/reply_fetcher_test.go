package github_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/delightfulhammers/bop/internal/adapter/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListPullRequestComments_Success(t *testing.T) {
	// Setup mock server
	comments := []github.PullRequestComment{
		{
			ID:   1,
			Body: "Original comment<!-- CR_FINGERPRINT:abc123 -->",
			User: github.User{Login: "code-reviewer[bot]", Type: "Bot"},
		},
		{
			ID:          2,
			Body:        "acknowledged - this is intentional",
			User:        github.User{Login: "developer", Type: "User"},
			InReplyToID: 1, // Reply to comment 1
		},
		{
			ID:   3,
			Body: "Another top-level comment<!-- CR_FINGERPRINT:def456 -->",
			User: github.User{Login: "code-reviewer[bot]", Type: "Bot"},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/owner/repo/pulls/42/comments", r.URL.Path)
		assert.Equal(t, "100", r.URL.Query().Get("per_page"))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(comments)
	}))
	defer server.Close()

	client := github.NewClient("test-token")
	client.SetBaseURL(server.URL)

	result, err := client.ListPullRequestComments(context.Background(), "owner", "repo", 42)

	require.NoError(t, err)
	require.Len(t, result, 3)
	assert.Equal(t, int64(1), result[0].ID)
	assert.Equal(t, int64(2), result[1].ID)
	assert.Equal(t, int64(1), result[1].InReplyToID)
}

func TestListPullRequestComments_Pagination(t *testing.T) {
	page := 0
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if page == 0 {
			// First page with Link header - use full URL
			w.Header().Set("Link", `<`+serverURL+`/repos/owner/repo/pulls/42/comments?page=2>; rel="next"`)
			_ = json.NewEncoder(w).Encode([]github.PullRequestComment{
				{ID: 1, Body: "Comment 1"},
			})
		} else {
			// Second page (no Link header = last page)
			_ = json.NewEncoder(w).Encode([]github.PullRequestComment{
				{ID: 2, Body: "Comment 2"},
			})
		}
		page++
	}))
	defer server.Close()

	serverURL = server.URL
	client := github.NewClient("test-token")
	client.SetBaseURL(server.URL)

	result, err := client.ListPullRequestComments(context.Background(), "owner", "repo", 42)

	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, int64(1), result[0].ID)
	assert.Equal(t, int64(2), result[1].ID)
}

func TestListPullRequestComments_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]github.PullRequestComment{})
	}))
	defer server.Close()

	client := github.NewClient("test-token")
	client.SetBaseURL(server.URL)

	result, err := client.ListPullRequestComments(context.Background(), "owner", "repo", 42)

	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestListPullRequestComments_InvalidOwner(t *testing.T) {
	client := github.NewClient("test-token")

	_, err := client.ListPullRequestComments(context.Background(), "../evil", "repo", 42)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "owner")
}

func TestGroupCommentsByParent(t *testing.T) {
	comments := []github.PullRequestComment{
		{ID: 1, Body: "Parent 1<!-- CR_FINGERPRINT:fp1 -->", User: github.User{Login: "bot", Type: "Bot"}},
		{ID: 2, Body: "Reply to 1", InReplyToID: 1, User: github.User{Login: "dev", Type: "User"}},
		{ID: 3, Body: "Another reply to 1", InReplyToID: 1, User: github.User{Login: "dev2", Type: "User"}},
		{ID: 4, Body: "Parent 2<!-- CR_FINGERPRINT:fp2 -->", User: github.User{Login: "bot", Type: "Bot"}},
		{ID: 5, Body: "Reply to 4", InReplyToID: 4, User: github.User{Login: "dev", Type: "User"}},
		{ID: 6, Body: "Orphan reply (parent not in list)", InReplyToID: 999},
	}

	botUsername := "bot"
	grouped := github.GroupCommentsByParent(comments, botUsername)

	// Should have 2 groups (parents 1 and 4)
	require.Len(t, grouped, 2)

	// Find the group for parent 1
	var group1 *github.CommentWithReplies
	for i := range grouped {
		if grouped[i].Parent.ID == 1 {
			group1 = &grouped[i]
			break
		}
	}
	require.NotNil(t, group1)
	assert.Len(t, group1.Replies, 2)

	// Find the group for parent 4
	var group4 *github.CommentWithReplies
	for i := range grouped {
		if grouped[i].Parent.ID == 4 {
			group4 = &grouped[i]
			break
		}
	}
	require.NotNil(t, group4)
	assert.Len(t, group4.Replies, 1)
}

func TestGroupCommentsByParent_NoParents(t *testing.T) {
	comments := []github.PullRequestComment{
		{ID: 1, Body: "Not a bot comment", User: github.User{Login: "human", Type: "User"}},
		{ID: 2, Body: "Reply", InReplyToID: 1, User: github.User{Login: "dev", Type: "User"}},
	}

	grouped := github.GroupCommentsByParent(comments, "bot")

	// No bot comments, so no groups
	assert.Empty(t, grouped)
}

func TestGroupCommentsByParent_BotRepliesExcluded(t *testing.T) {
	// Bot replying to itself shouldn't be treated as a status update
	comments := []github.PullRequestComment{
		{ID: 1, Body: "Parent<!-- CR_FINGERPRINT:fp1 -->", User: github.User{Login: "bot", Type: "Bot"}},
		{ID: 2, Body: "Bot follow-up", InReplyToID: 1, User: github.User{Login: "bot", Type: "Bot"}},
		{ID: 3, Body: "Human reply", InReplyToID: 1, User: github.User{Login: "dev", Type: "User"}},
	}

	grouped := github.GroupCommentsByParent(comments, "bot")

	require.Len(t, grouped, 1)
	// Only the human reply should be included
	assert.Len(t, grouped[0].Replies, 1)
	assert.Equal(t, "Human reply", grouped[0].Replies[0].Body)
}
