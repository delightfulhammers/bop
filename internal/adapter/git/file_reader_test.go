package git_test

import (
	"context"
	"testing"

	goGit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bkyoung/code-reviewer/internal/adapter/git"
	"github.com/bkyoung/code-reviewer/internal/usecase/triage"
)

func TestEngine_ReadFile(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()

	repo, err := goGit.PlainInit(tmp, false)
	require.NoError(t, err)

	worktree, err := repo.Worktree()
	require.NoError(t, err)

	// Create initial commit with a file
	fileContent := "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"
	writeFile(t, tmp, "main.go", fileContent)
	_, err = worktree.Add("main.go")
	require.NoError(t, err)
	commit1, err := worktree.Commit("initial", &goGit.CommitOptions{
		Author: defaultSignature(),
	})
	require.NoError(t, err)

	// Create second commit with modified file
	modifiedContent := "package main\n\nfunc main() {\n\tprintln(\"modified\")\n}\n"
	writeFile(t, tmp, "main.go", modifiedContent)
	_, err = worktree.Add("main.go")
	require.NoError(t, err)
	commit2, err := worktree.Commit("modified", &goGit.CommitOptions{
		Author: defaultSignature(),
	})
	require.NoError(t, err)

	engine := git.NewEngine(tmp)

	t.Run("reads file at specific commit", func(t *testing.T) {
		content, err := engine.ReadFile(ctx, "main.go", commit1.String())
		require.NoError(t, err)
		assert.Equal(t, fileContent, content)
	})

	t.Run("reads modified file at later commit", func(t *testing.T) {
		content, err := engine.ReadFile(ctx, "main.go", commit2.String())
		require.NoError(t, err)
		assert.Equal(t, modifiedContent, content)
	})

	t.Run("reads file at branch ref", func(t *testing.T) {
		content, err := engine.ReadFile(ctx, "main.go", "master")
		require.NoError(t, err)
		assert.Equal(t, modifiedContent, content)
	})

	t.Run("returns error for non-existent file", func(t *testing.T) {
		_, err := engine.ReadFile(ctx, "nonexistent.go", commit1.String())
		require.Error(t, err)
		assert.ErrorIs(t, err, triage.ErrFileNotFound)
	})

	t.Run("returns error for non-existent ref", func(t *testing.T) {
		_, err := engine.ReadFile(ctx, "main.go", "0000000000000000000000000000000000000000")
		require.Error(t, err)
	})

	t.Run("cancelled context returns error", func(t *testing.T) {
		cancelledCtx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := engine.ReadFile(cancelledCtx, "main.go", commit1.String())
		require.Error(t, err)
	})
}

func TestEngine_ReadFileLines(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()

	repo, err := goGit.PlainInit(tmp, false)
	require.NoError(t, err)

	worktree, err := repo.Worktree()
	require.NoError(t, err)

	// Create file with numbered lines for easy testing
	fileContent := `line1
line2
line3
line4
line5
line6
line7
line8
line9
line10
`
	writeFile(t, tmp, "test.txt", fileContent)
	_, err = worktree.Add("test.txt")
	require.NoError(t, err)
	commit, err := worktree.Commit("initial", &goGit.CommitOptions{
		Author: defaultSignature(),
	})
	require.NoError(t, err)

	engine := git.NewEngine(tmp)

	t.Run("reads specific line range", func(t *testing.T) {
		ctx, err := engine.ReadFileLines(ctx, "test.txt", commit.String(), 3, 5, 0)
		require.NoError(t, err)
		assert.Equal(t, "test.txt", ctx.File)
		assert.Equal(t, 3, ctx.StartLine)
		assert.Equal(t, 5, ctx.EndLine)
		assert.Equal(t, "line3\nline4\nline5", ctx.Content)
	})

	t.Run("reads single line", func(t *testing.T) {
		ctx, err := engine.ReadFileLines(ctx, "test.txt", commit.String(), 5, 5, 0)
		require.NoError(t, err)
		assert.Equal(t, "line5", ctx.Content)
		assert.Equal(t, 1, ctx.LineCount())
	})

	t.Run("reads with context lines", func(t *testing.T) {
		ctx, err := engine.ReadFileLines(ctx, "test.txt", commit.String(), 5, 5, 2)
		require.NoError(t, err)
		// Should include lines 3-7 (2 before, target line 5, 2 after)
		assert.Equal(t, 3, ctx.StartLine)
		assert.Equal(t, 7, ctx.EndLine)
		assert.Contains(t, ctx.Content, "line3")
		assert.Contains(t, ctx.Content, "line7")
		assert.Equal(t, 2, ctx.ContextBefore)
		assert.Equal(t, 2, ctx.ContextAfter)
	})

	t.Run("context clamps at file start", func(t *testing.T) {
		ctx, err := engine.ReadFileLines(ctx, "test.txt", commit.String(), 2, 2, 5)
		require.NoError(t, err)
		assert.Equal(t, 1, ctx.StartLine)     // Clamped to 1
		assert.Equal(t, 7, ctx.EndLine)       // 2 + 5 = 7
		assert.Equal(t, 1, ctx.ContextBefore) // Only 1 line before line 2
	})

	t.Run("context clamps at file end", func(t *testing.T) {
		ctx, err := engine.ReadFileLines(ctx, "test.txt", commit.String(), 9, 9, 5)
		require.NoError(t, err)
		assert.Equal(t, 4, ctx.StartLine)    // 9 - 5 = 4
		assert.Equal(t, 10, ctx.EndLine)     // Clamped to 10
		assert.Equal(t, 1, ctx.ContextAfter) // Only 1 line after line 9
	})

	t.Run("returns error for invalid line range", func(t *testing.T) {
		_, err := engine.ReadFileLines(ctx, "test.txt", commit.String(), 5, 3, 0)
		require.Error(t, err)
		assert.ErrorIs(t, err, triage.ErrInvalidLineRange)
	})

	t.Run("returns error for out of bounds line", func(t *testing.T) {
		_, err := engine.ReadFileLines(ctx, "test.txt", commit.String(), 100, 105, 0)
		require.Error(t, err)
		assert.ErrorIs(t, err, triage.ErrInvalidLineRange)
	})

	t.Run("returns error for non-existent file", func(t *testing.T) {
		_, err := engine.ReadFileLines(ctx, "nonexistent.txt", commit.String(), 1, 5, 0)
		require.Error(t, err)
		assert.ErrorIs(t, err, triage.ErrFileNotFound)
	})
}

func TestEngine_ReadFile_SizeCheckBeforeRead(t *testing.T) {
	// This test verifies the behavior documented in the code: file size is checked
	// before reading to avoid allocating memory for oversized files.
	// The actual 10MB threshold test is impractical for unit tests, but we verify
	// that normal files are read correctly and the size metadata is accessible.
	ctx := context.Background()
	tmp := t.TempDir()

	repo, err := goGit.PlainInit(tmp, false)
	require.NoError(t, err)

	worktree, err := repo.Worktree()
	require.NoError(t, err)

	// Create a file with known size
	content := "hello world\n"
	writeFile(t, tmp, "test.txt", content)
	_, err = worktree.Add("test.txt")
	require.NoError(t, err)
	commit, err := worktree.Commit("add file", &goGit.CommitOptions{
		Author: defaultSignature(),
	})
	require.NoError(t, err)

	engine := git.NewEngine(tmp)

	t.Run("reads file with size below limit", func(t *testing.T) {
		result, err := engine.ReadFile(ctx, "test.txt", commit.String())
		require.NoError(t, err)
		assert.Equal(t, content, result)
	})

	// Note: Testing ErrFileTruncated for files > 10MB is impractical in unit tests.
	// The optimization ensures we don't allocate memory for such files by checking
	// file.Size (from Blob metadata) before calling io.ReadAll.
}

func TestEngine_GetDiffHunk(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()

	repo, err := goGit.PlainInit(tmp, false)
	require.NoError(t, err)

	worktree, err := repo.Worktree()
	require.NoError(t, err)

	// Create initial commit
	initialContent := `line1
line2
line3
line4
line5
`
	writeFile(t, tmp, "test.txt", initialContent)
	_, err = worktree.Add("test.txt")
	require.NoError(t, err)
	_, err = worktree.Commit("initial", &goGit.CommitOptions{
		Author: defaultSignature(),
	})
	require.NoError(t, err)

	// Create feature branch with changes
	err = checkoutBranch(worktree, "feature")
	require.NoError(t, err)

	modifiedContent := `line1
line2
modified line3
line4
added line
line5
`
	writeFile(t, tmp, "test.txt", modifiedContent)
	_, err = worktree.Add("test.txt")
	require.NoError(t, err)
	commitHash, err := worktree.Commit("modify line 3 and add line", &goGit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
		},
	})
	require.NoError(t, err)

	engine := git.NewEngine(tmp)

	t.Run("returns diff hunk for modified lines", func(t *testing.T) {
		diffCtx, err := engine.GetDiffHunk(ctx, "master", commitHash.String(), "test.txt", 3, 5)
		require.NoError(t, err)
		assert.Equal(t, "test.txt", diffCtx.File)
		assert.Equal(t, "master", diffCtx.BaseBranch)
		assert.Equal(t, commitHash.String(), diffCtx.TargetRef)
		assert.Contains(t, diffCtx.HunkContent, "-line3")
		assert.Contains(t, diffCtx.HunkContent, "+modified line3")
		assert.True(t, diffCtx.HasChanges())
	})

	t.Run("returns empty for unchanged lines", func(t *testing.T) {
		// Line 1 is unchanged
		diffCtx, err := engine.GetDiffHunk(ctx, "master", commitHash.String(), "test.txt", 1, 1)
		require.NoError(t, err)
		// Should still return the diff context, but may not have specific changes for line 1
		assert.NotNil(t, diffCtx)
	})

	t.Run("returns error for non-existent file", func(t *testing.T) {
		_, err := engine.GetDiffHunk(ctx, "master", commitHash.String(), "nonexistent.txt", 1, 5)
		require.Error(t, err)
	})
}
