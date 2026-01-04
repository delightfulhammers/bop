package repository_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/delightfulhammers/bop/internal/adapter/repository"
	"github.com/delightfulhammers/bop/internal/usecase/verify"
)

// Compile-time check that GitRepository implements verify.Repository.
var _ verify.Repository = (*repository.GitRepository)(nil)

func TestGitRepository_RespectsGitignore(t *testing.T) {
	tmp := t.TempDir()

	// Initialize a git repo
	gitDir := filepath.Join(tmp, ".git")
	if err := os.Mkdir(gitDir, 0o755); err != nil {
		t.Fatalf("failed to create .git dir: %v", err)
	}

	// Create .gitignore
	gitignore := `*.log
node_modules/
build/
`
	if err := os.WriteFile(filepath.Join(tmp, ".gitignore"), []byte(gitignore), 0o644); err != nil {
		t.Fatalf("failed to write .gitignore: %v", err)
	}

	// Create files - some tracked, some ignored
	trackedFiles := []string{"main.go", "util.go", "README.md"}
	ignoredFiles := []string{"app.log", "debug.log"}
	ignoredDirs := []string{"node_modules/package.json", "build/output.js"}

	for _, f := range trackedFiles {
		if err := os.WriteFile(filepath.Join(tmp, f), []byte("content"), 0o644); err != nil {
			t.Fatalf("failed to write %s: %v", f, err)
		}
	}

	for _, f := range ignoredFiles {
		if err := os.WriteFile(filepath.Join(tmp, f), []byte("log"), 0o644); err != nil {
			t.Fatalf("failed to write %s: %v", f, err)
		}
	}

	for _, f := range ignoredDirs {
		dir := filepath.Dir(filepath.Join(tmp, f))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("failed to create dir for %s: %v", f, err)
		}
		if err := os.WriteFile(filepath.Join(tmp, f), []byte("content"), 0o644); err != nil {
			t.Fatalf("failed to write %s: %v", f, err)
		}
	}

	repo := repository.NewGitRepository(tmp)

	t.Run("Glob excludes ignored files", func(t *testing.T) {
		matches, err := repo.Glob("*")
		if err != nil {
			t.Fatalf("Glob error: %v", err)
		}

		matchSet := make(map[string]bool)
		for _, m := range matches {
			matchSet[m] = true
		}

		// Check tracked files are included
		for _, f := range trackedFiles {
			if !matchSet[f] {
				t.Errorf("tracked file %q not in glob results", f)
			}
		}

		// Check ignored files are excluded
		for _, f := range ignoredFiles {
			if matchSet[f] {
				t.Errorf("ignored file %q should not be in glob results", f)
			}
		}
	})

	t.Run("Grep excludes ignored files", func(t *testing.T) {
		matches, err := repo.Grep("content")
		if err != nil {
			t.Fatalf("Grep error: %v", err)
		}

		matchedFiles := make(map[string]bool)
		for _, m := range matches {
			matchedFiles[m.File] = true
		}

		// Check tracked files are searchable
		for _, f := range trackedFiles {
			if !matchedFiles[f] {
				t.Errorf("tracked file %q not found in grep results", f)
			}
		}

		// Check ignored directories are excluded
		for _, f := range ignoredDirs {
			if matchedFiles[f] {
				t.Errorf("ignored file %q should not be in grep results", f)
			}
		}
	})

	t.Run("ReadFile still works on ignored files", func(t *testing.T) {
		// ReadFile should work even on ignored files (explicit access)
		content, err := repo.ReadFile("app.log")
		if err != nil {
			t.Errorf("ReadFile should work on ignored file: %v", err)
		}
		if string(content) != "log" {
			t.Errorf("got %q, want %q", content, "log")
		}
	})

	t.Run("FileExists works on ignored files", func(t *testing.T) {
		if !repo.FileExists("app.log") {
			t.Error("FileExists should return true for ignored file")
		}
	})
}

func TestGitRepository_FallsBackToLocalForNonGitDir(t *testing.T) {
	tmp := t.TempDir()

	// Create a file (no .git directory)
	if err := os.WriteFile(filepath.Join(tmp, "test.go"), []byte("package main"), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	repo := repository.NewGitRepository(tmp)

	t.Run("ReadFile works without git", func(t *testing.T) {
		content, err := repo.ReadFile("test.go")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if string(content) != "package main" {
			t.Errorf("got %q, want %q", content, "package main")
		}
	})

	t.Run("Glob works without git", func(t *testing.T) {
		matches, err := repo.Glob("*.go")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(matches) != 1 {
			t.Errorf("got %d matches, want 1", len(matches))
		}
	})
}

func TestGitRepository_RunCommand(t *testing.T) {
	tmp := t.TempDir()

	// Initialize a git repo
	gitDir := filepath.Join(tmp, ".git")
	if err := os.Mkdir(gitDir, 0o755); err != nil {
		t.Fatalf("failed to create .git dir: %v", err)
	}

	repo := repository.NewGitRepository(tmp)

	t.Run("inherits RunCommand from LocalRepository", func(t *testing.T) {
		var result verify.CommandResult
		var err error
		if runtime.GOOS == "windows" {
			result, err = repo.RunCommand(context.Background(), "cmd", "/c", "echo", "test")
		} else {
			result, err = repo.RunCommand(context.Background(), "echo", "test")
		}
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result.ExitCode != 0 {
			t.Errorf("got exit code %d, want 0", result.ExitCode)
		}
	})
}

func TestNewGitRepository(t *testing.T) {
	tmp := t.TempDir()
	repo := repository.NewGitRepository(tmp)

	if repo == nil {
		t.Fatal("expected non-nil repository")
	}
}
