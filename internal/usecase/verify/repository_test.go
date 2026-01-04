package verify_test

import (
	"context"
	"testing"

	"github.com/delightfulhammers/bop/internal/usecase/verify"
)

// mockRepository implements verify.Repository for testing.
type mockRepository struct {
	readFileFunc   func(path string) ([]byte, error)
	fileExistsFunc func(path string) bool
	globFunc       func(pattern string) ([]string, error)
	grepFunc       func(pattern string, paths ...string) ([]verify.GrepMatch, error)
	runCommandFunc func(ctx context.Context, cmd string, args ...string) (verify.CommandResult, error)
}

func (m *mockRepository) ReadFile(path string) ([]byte, error) {
	if m.readFileFunc != nil {
		return m.readFileFunc(path)
	}
	return nil, nil
}

func (m *mockRepository) FileExists(path string) bool {
	if m.fileExistsFunc != nil {
		return m.fileExistsFunc(path)
	}
	return false
}

func (m *mockRepository) Glob(pattern string) ([]string, error) {
	if m.globFunc != nil {
		return m.globFunc(pattern)
	}
	return nil, nil
}

func (m *mockRepository) Grep(pattern string, paths ...string) ([]verify.GrepMatch, error) {
	if m.grepFunc != nil {
		return m.grepFunc(pattern, paths...)
	}
	return nil, nil
}

func (m *mockRepository) RunCommand(ctx context.Context, cmd string, args ...string) (verify.CommandResult, error) {
	if m.runCommandFunc != nil {
		return m.runCommandFunc(ctx, cmd, args...)
	}
	return verify.CommandResult{}, nil
}

// Compile-time check that mockRepository implements Repository.
var _ verify.Repository = (*mockRepository)(nil)

func TestRepository_Interface(t *testing.T) {
	t.Run("ReadFile returns content", func(t *testing.T) {
		repo := &mockRepository{
			readFileFunc: func(path string) ([]byte, error) {
				return []byte("content"), nil
			},
		}
		content, err := repo.ReadFile("test.go")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if string(content) != "content" {
			t.Errorf("got %q, want %q", content, "content")
		}
	})

	t.Run("FileExists returns true for existing file", func(t *testing.T) {
		repo := &mockRepository{
			fileExistsFunc: func(path string) bool {
				return path == "exists.go"
			},
		}
		if !repo.FileExists("exists.go") {
			t.Error("expected FileExists to return true for exists.go")
		}
		if repo.FileExists("missing.go") {
			t.Error("expected FileExists to return false for missing.go")
		}
	})

	t.Run("Glob returns matching files", func(t *testing.T) {
		repo := &mockRepository{
			globFunc: func(pattern string) ([]string, error) {
				return []string{"a.go", "b.go"}, nil
			},
		}
		files, err := repo.Glob("*.go")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(files) != 2 {
			t.Errorf("got %d files, want 2", len(files))
		}
	})

	t.Run("Grep returns matches", func(t *testing.T) {
		repo := &mockRepository{
			grepFunc: func(pattern string, paths ...string) ([]verify.GrepMatch, error) {
				return []verify.GrepMatch{
					{File: "test.go", Line: 10, Content: "func Test()"},
				}, nil
			},
		}
		matches, err := repo.Grep("func.*Test", "test.go")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(matches) != 1 {
			t.Errorf("got %d matches, want 1", len(matches))
		}
		if matches[0].Line != 10 {
			t.Errorf("got line %d, want 10", matches[0].Line)
		}
	})

	t.Run("RunCommand returns result", func(t *testing.T) {
		repo := &mockRepository{
			runCommandFunc: func(ctx context.Context, cmd string, args ...string) (verify.CommandResult, error) {
				return verify.CommandResult{
					Stdout:   "output",
					Stderr:   "",
					ExitCode: 0,
				}, nil
			},
		}
		result, err := repo.RunCommand(context.Background(), "go", "build")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result.Stdout != "output" {
			t.Errorf("got stdout %q, want %q", result.Stdout, "output")
		}
		if result.ExitCode != 0 {
			t.Errorf("got exit code %d, want 0", result.ExitCode)
		}
	})
}

func TestGrepMatch_Struct(t *testing.T) {
	match := verify.GrepMatch{
		File:    "main.go",
		Line:    42,
		Content: "package main",
	}

	if match.File != "main.go" {
		t.Errorf("got File %q, want %q", match.File, "main.go")
	}
	if match.Line != 42 {
		t.Errorf("got Line %d, want 42", match.Line)
	}
	if match.Content != "package main" {
		t.Errorf("got Content %q, want %q", match.Content, "package main")
	}
}

func TestCommandResult_Struct(t *testing.T) {
	result := verify.CommandResult{
		Stdout:   "hello",
		Stderr:   "warning",
		ExitCode: 1,
	}

	if result.Stdout != "hello" {
		t.Errorf("got Stdout %q, want %q", result.Stdout, "hello")
	}
	if result.Stderr != "warning" {
		t.Errorf("got Stderr %q, want %q", result.Stderr, "warning")
	}
	if result.ExitCode != 1 {
		t.Errorf("got ExitCode %d, want 1", result.ExitCode)
	}
}

func TestCommandResult_Success(t *testing.T) {
	t.Run("returns true for exit code 0", func(t *testing.T) {
		result := verify.CommandResult{ExitCode: 0}
		if !result.Success() {
			t.Error("expected Success() to return true for exit code 0")
		}
	})

	t.Run("returns false for non-zero exit code", func(t *testing.T) {
		result := verify.CommandResult{ExitCode: 1}
		if result.Success() {
			t.Error("expected Success() to return false for exit code 1")
		}
	})
}
