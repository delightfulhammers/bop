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

// Compile-time check that LocalRepository implements verify.Repository.
var _ verify.Repository = (*repository.LocalRepository)(nil)

func TestLocalRepository_ReadFile(t *testing.T) {
	tmp := t.TempDir()
	repo := repository.NewLocalRepository(tmp)

	t.Run("reads existing file", func(t *testing.T) {
		content := "package main\n\nfunc main() {}\n"
		path := filepath.Join(tmp, "main.go")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		result, err := repo.ReadFile("main.go")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if string(result) != content {
			t.Errorf("got %q, want %q", result, content)
		}
	})

	t.Run("returns error for missing file", func(t *testing.T) {
		_, err := repo.ReadFile("nonexistent.go")
		if err == nil {
			t.Error("expected error for missing file")
		}
	})

	t.Run("prevents path traversal", func(t *testing.T) {
		_, err := repo.ReadFile("../../../etc/passwd")
		if err == nil {
			t.Error("expected error for path traversal attempt")
		}
	})

	t.Run("handles absolute path within root", func(t *testing.T) {
		content := "test content"
		path := filepath.Join(tmp, "test.txt")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		result, err := repo.ReadFile(path)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if string(result) != content {
			t.Errorf("got %q, want %q", result, content)
		}
	})
}

func TestLocalRepository_FileExists(t *testing.T) {
	tmp := t.TempDir()
	repo := repository.NewLocalRepository(tmp)

	t.Run("returns true for existing file", func(t *testing.T) {
		path := filepath.Join(tmp, "exists.go")
		if err := os.WriteFile(path, []byte("package main"), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		if !repo.FileExists("exists.go") {
			t.Error("expected FileExists to return true for existing file")
		}
	})

	t.Run("returns false for missing file", func(t *testing.T) {
		if repo.FileExists("missing.go") {
			t.Error("expected FileExists to return false for missing file")
		}
	})

	t.Run("returns false for directory", func(t *testing.T) {
		dir := filepath.Join(tmp, "subdir")
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatalf("failed to create test dir: %v", err)
		}

		if repo.FileExists("subdir") {
			t.Error("expected FileExists to return false for directory")
		}
	})

	t.Run("returns false for path traversal", func(t *testing.T) {
		if repo.FileExists("../../../etc/passwd") {
			t.Error("expected FileExists to return false for path traversal")
		}
	})
}

func TestLocalRepository_Glob(t *testing.T) {
	tmp := t.TempDir()
	repo := repository.NewLocalRepository(tmp)

	// Create test files
	files := []string{"main.go", "util.go", "README.md", "sub/helper.go"}
	for _, f := range files {
		path := filepath.Join(tmp, f)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
	}

	t.Run("matches simple pattern", func(t *testing.T) {
		matches, err := repo.Glob("*.go")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(matches) != 2 {
			t.Errorf("got %d matches, want 2: %v", len(matches), matches)
		}
	})

	t.Run("matches recursive pattern", func(t *testing.T) {
		matches, err := repo.Glob("**/*.go")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(matches) < 2 {
			t.Errorf("got %d matches, want at least 2: %v", len(matches), matches)
		}
	})

	t.Run("returns empty for no matches", func(t *testing.T) {
		matches, err := repo.Glob("*.xyz")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(matches) != 0 {
			t.Errorf("got %d matches, want 0", len(matches))
		}
	})
}

func TestLocalRepository_Grep(t *testing.T) {
	tmp := t.TempDir()
	repo := repository.NewLocalRepository(tmp)

	// Create test files
	mainGo := `package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`
	utilGo := `package main

func helper() string {
	return "helper"
}
`
	if err := os.WriteFile(filepath.Join(tmp, "main.go"), []byte(mainGo), 0o644); err != nil {
		t.Fatalf("failed to write main.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "util.go"), []byte(utilGo), 0o644); err != nil {
		t.Fatalf("failed to write util.go: %v", err)
	}

	t.Run("finds pattern in file", func(t *testing.T) {
		matches, err := repo.Grep("fmt\\.Println", "main.go")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(matches) != 1 {
			t.Errorf("got %d matches, want 1: %v", len(matches), matches)
		}
		if len(matches) > 0 && matches[0].Line != 6 {
			t.Errorf("got line %d, want 6", matches[0].Line)
		}
	})

	t.Run("finds pattern across files", func(t *testing.T) {
		matches, err := repo.Grep("func ")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(matches) < 2 {
			t.Errorf("got %d matches, want at least 2", len(matches))
		}
	})

	t.Run("returns empty for no matches", func(t *testing.T) {
		matches, err := repo.Grep("NONEXISTENT_PATTERN")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(matches) != 0 {
			t.Errorf("got %d matches, want 0", len(matches))
		}
	})
}

func TestLocalRepository_RunCommand(t *testing.T) {
	tmp := t.TempDir()
	repo := repository.NewLocalRepository(tmp)

	t.Run("runs command successfully", func(t *testing.T) {
		var result verify.CommandResult
		var err error

		if runtime.GOOS == "windows" {
			result, err = repo.RunCommand(context.Background(), "cmd", "/c", "echo", "hello")
		} else {
			result, err = repo.RunCommand(context.Background(), "echo", "hello")
		}

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result.ExitCode != 0 {
			t.Errorf("got exit code %d, want 0", result.ExitCode)
		}
		if !result.Success() {
			t.Error("expected Success() to return true")
		}
	})

	t.Run("captures exit code on failure", func(t *testing.T) {
		var result verify.CommandResult
		var err error

		if runtime.GOOS == "windows" {
			result, err = repo.RunCommand(context.Background(), "cmd", "/c", "exit", "1")
		} else {
			result, err = repo.RunCommand(context.Background(), "sh", "-c", "exit 1")
		}

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result.ExitCode != 1 {
			t.Errorf("got exit code %d, want 1", result.ExitCode)
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		var err error
		if runtime.GOOS == "windows" {
			_, err = repo.RunCommand(ctx, "cmd", "/c", "ping", "-n", "10", "127.0.0.1")
		} else {
			_, err = repo.RunCommand(ctx, "sleep", "10")
		}
		if err == nil {
			t.Error("expected error for cancelled context")
		}
	})

	t.Run("returns error for nonexistent command", func(t *testing.T) {
		_, err := repo.RunCommand(context.Background(), "nonexistent_command_xyz123")
		if err == nil {
			t.Error("expected error for nonexistent command")
		}
	})
}

func TestNewLocalRepository(t *testing.T) {
	tmp := t.TempDir()
	repo := repository.NewLocalRepository(tmp)

	if repo == nil {
		t.Fatal("expected non-nil repository")
	}

	// Verify we can use it
	path := filepath.Join(tmp, "test.txt")
	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	if !repo.FileExists("test.txt") {
		t.Error("expected FileExists to return true for test file")
	}
}
