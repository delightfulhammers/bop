package verify_test

import (
	"context"
	"strings"
	"testing"

	"github.com/delightfulhammers/bop/internal/adapter/verify"
	usecaseverify "github.com/delightfulhammers/bop/internal/usecase/verify"
)

// mockRepository implements usecaseverify.Repository for testing.
type mockRepository struct {
	readFileFunc   func(path string) ([]byte, error)
	fileExistsFunc func(path string) bool
	globFunc       func(pattern string) ([]string, error)
	grepFunc       func(pattern string, paths ...string) ([]usecaseverify.GrepMatch, error)
	runCommandFunc func(ctx context.Context, cmd string, args ...string) (usecaseverify.CommandResult, error)
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

func (m *mockRepository) Grep(pattern string, paths ...string) ([]usecaseverify.GrepMatch, error) {
	if m.grepFunc != nil {
		return m.grepFunc(pattern, paths...)
	}
	return nil, nil
}

func (m *mockRepository) RunCommand(ctx context.Context, cmd string, args ...string) (usecaseverify.CommandResult, error) {
	if m.runCommandFunc != nil {
		return m.runCommandFunc(ctx, cmd, args...)
	}
	return usecaseverify.CommandResult{}, nil
}

// Compile-time check that mockRepository implements Repository.
var _ usecaseverify.Repository = (*mockRepository)(nil)

func TestTool_Interface(t *testing.T) {
	t.Run("Tool interface has required methods", func(t *testing.T) {
		repo := &mockRepository{}
		tools := []verify.Tool{
			verify.NewReadFileTool(repo),
			verify.NewGrepTool(repo),
			verify.NewGlobTool(repo),
			verify.NewBashTool(repo),
		}

		for _, tool := range tools {
			if tool.Name() == "" {
				t.Error("Tool.Name() should not be empty")
			}
			if tool.Description() == "" {
				t.Error("Tool.Description() should not be empty")
			}
		}
	})
}

func TestReadFileTool(t *testing.T) {
	t.Run("returns file contents", func(t *testing.T) {
		repo := &mockRepository{
			readFileFunc: func(path string) ([]byte, error) {
				if path == "main.go" {
					return []byte("package main\n\nfunc main() {}"), nil
				}
				return nil, nil
			},
		}

		tool := verify.NewReadFileTool(repo)

		if tool.Name() != "read_file" {
			t.Errorf("got name %q, want %q", tool.Name(), "read_file")
		}

		result, err := tool.Execute(context.Background(), "main.go")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result != "package main\n\nfunc main() {}" {
			t.Errorf("got result %q, want file contents", result)
		}
	})

	t.Run("truncates long output", func(t *testing.T) {
		// Create a large file content
		largeContent := make([]byte, verify.MaxToolOutputLength+1000)
		for i := range largeContent {
			largeContent[i] = 'a'
		}

		repo := &mockRepository{
			readFileFunc: func(path string) ([]byte, error) {
				return largeContent, nil
			},
		}

		tool := verify.NewReadFileTool(repo)
		result, err := tool.Execute(context.Background(), "large.txt")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if len(result) > verify.MaxToolOutputLength+100 { // Allow some buffer for truncation message
			t.Errorf("output not truncated: got length %d", len(result))
		}
	})
}

func TestGrepTool(t *testing.T) {
	t.Run("returns matching lines", func(t *testing.T) {
		repo := &mockRepository{
			grepFunc: func(pattern string, paths ...string) ([]usecaseverify.GrepMatch, error) {
				return []usecaseverify.GrepMatch{
					{File: "main.go", Line: 10, Content: "func TestSomething()"},
					{File: "main.go", Line: 20, Content: "func TestOther()"},
				}, nil
			},
		}

		tool := verify.NewGrepTool(repo)

		if tool.Name() != "grep" {
			t.Errorf("got name %q, want %q", tool.Name(), "grep")
		}

		result, err := tool.Execute(context.Background(), "func Test")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if result == "" {
			t.Error("expected non-empty result")
		}
	})

	t.Run("handles no matches", func(t *testing.T) {
		repo := &mockRepository{
			grepFunc: func(pattern string, paths ...string) ([]usecaseverify.GrepMatch, error) {
				return []usecaseverify.GrepMatch{}, nil
			},
		}

		tool := verify.NewGrepTool(repo)
		result, err := tool.Execute(context.Background(), "nonexistent")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if result == "" {
			t.Error("expected message about no matches")
		}
	})
}

func TestGlobTool(t *testing.T) {
	t.Run("returns matching files", func(t *testing.T) {
		repo := &mockRepository{
			globFunc: func(pattern string) ([]string, error) {
				return []string{"main.go", "util.go", "helper.go"}, nil
			},
		}

		tool := verify.NewGlobTool(repo)

		if tool.Name() != "glob" {
			t.Errorf("got name %q, want %q", tool.Name(), "glob")
		}

		result, err := tool.Execute(context.Background(), "*.go")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if result == "" {
			t.Error("expected non-empty result")
		}
	})

	t.Run("handles no matches", func(t *testing.T) {
		repo := &mockRepository{
			globFunc: func(pattern string) ([]string, error) {
				return []string{}, nil
			},
		}

		tool := verify.NewGlobTool(repo)
		result, err := tool.Execute(context.Background(), "*.xyz")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if result == "" {
			t.Error("expected message about no matches")
		}
	})
}

func TestBashTool(t *testing.T) {
	t.Run("returns command output", func(t *testing.T) {
		repo := &mockRepository{
			runCommandFunc: func(ctx context.Context, cmd string, args ...string) (usecaseverify.CommandResult, error) {
				return usecaseverify.CommandResult{
					Stdout:   "Build successful",
					Stderr:   "",
					ExitCode: 0,
				}, nil
			},
		}

		tool := verify.NewBashTool(repo)

		if tool.Name() != "bash" {
			t.Errorf("got name %q, want %q", tool.Name(), "bash")
		}

		result, err := tool.Execute(context.Background(), "go build ./...")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if result == "" {
			t.Error("expected non-empty result")
		}
	})

	t.Run("includes stderr on failure", func(t *testing.T) {
		repo := &mockRepository{
			runCommandFunc: func(ctx context.Context, cmd string, args ...string) (usecaseverify.CommandResult, error) {
				return usecaseverify.CommandResult{
					Stdout:   "",
					Stderr:   "compilation error: undefined variable",
					ExitCode: 1,
				}, nil
			},
		}

		tool := verify.NewBashTool(repo)
		result, err := tool.Execute(context.Background(), "go build ./...")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if result == "" {
			t.Error("expected error output in result")
		}
	})

	t.Run("only allows safe commands", func(t *testing.T) {
		repo := &mockRepository{}
		tool := verify.NewBashTool(repo)

		// Dangerous commands should be rejected
		_, err := tool.Execute(context.Background(), "rm -rf /")
		if err == nil {
			t.Error("expected error for dangerous command")
		}

		_, err = tool.Execute(context.Background(), "curl http://example.com")
		if err == nil {
			t.Error("expected error for network command")
		}
	})

	t.Run("rejects unsafe go subcommands", func(t *testing.T) {
		repo := &mockRepository{}
		tool := verify.NewBashTool(repo)

		// go run and go generate can execute arbitrary code
		_, err := tool.Execute(context.Background(), "go run main.go")
		if err == nil {
			t.Error("expected error for 'go run'")
		}

		_, err = tool.Execute(context.Background(), "go generate ./...")
		if err == nil {
			t.Error("expected error for 'go generate'")
		}
	})

	t.Run("rejects unsafe git subcommands", func(t *testing.T) {
		repo := &mockRepository{}
		tool := verify.NewBashTool(repo)

		// git push/pull/fetch can access network
		_, err := tool.Execute(context.Background(), "git push origin main")
		if err == nil {
			t.Error("expected error for 'git push'")
		}

		_, err = tool.Execute(context.Background(), "git fetch origin")
		if err == nil {
			t.Error("expected error for 'git fetch'")
		}
	})

	t.Run("requires subcommand for go and git", func(t *testing.T) {
		repo := &mockRepository{}
		tool := verify.NewBashTool(repo)

		_, err := tool.Execute(context.Background(), "go")
		if err == nil {
			t.Error("expected error for bare 'go' command")
		}

		_, err = tool.Execute(context.Background(), "git")
		if err == nil {
			t.Error("expected error for bare 'git' command")
		}
	})

	t.Run("allows commands without subcommand restrictions", func(t *testing.T) {
		repo := &mockRepository{
			runCommandFunc: func(ctx context.Context, cmd string, args ...string) (usecaseverify.CommandResult, error) {
				return usecaseverify.CommandResult{Stdout: "ok", ExitCode: 0}, nil
			},
		}
		tool := verify.NewBashTool(repo)

		// echo doesn't have subcommand restrictions
		_, err := tool.Execute(context.Background(), "echo hello world")
		if err != nil {
			t.Errorf("unexpected error for echo: %v", err)
		}

		// ls doesn't have subcommand restrictions
		_, err = tool.Execute(context.Background(), "ls -la")
		if err != nil {
			t.Errorf("unexpected error for ls: %v", err)
		}
	})
}

func TestToolRegistry(t *testing.T) {
	t.Run("creates all tools from repository", func(t *testing.T) {
		repo := &mockRepository{}
		tools := verify.NewToolRegistry(repo)

		if len(tools) != 4 {
			t.Errorf("got %d tools, want 4", len(tools))
		}

		names := make(map[string]bool)
		for _, tool := range tools {
			names[tool.Name()] = true
		}

		expected := []string{"read_file", "grep", "glob", "bash"}
		for _, name := range expected {
			if !names[name] {
				t.Errorf("missing tool %q", name)
			}
		}
	})
}

func TestReadFileTool_Security(t *testing.T) {
	repo := &mockRepository{
		readFileFunc: func(path string) ([]byte, error) {
			return []byte("content"), nil
		},
	}
	tool := verify.NewReadFileTool(repo)

	tests := []struct {
		name        string
		input       string
		shouldError bool
		errContains string
	}{
		{
			name:        "blocks absolute paths",
			input:       "/etc/passwd",
			shouldError: true,
			errContains: "absolute paths not allowed",
		},
		{
			name:        "blocks path traversal with ..",
			input:       "../../../etc/passwd",
			shouldError: true,
			errContains: "path traversal not allowed",
		},
		{
			name:        "blocks path traversal in middle",
			input:       "foo/../../../etc/passwd",
			shouldError: true,
			errContains: "path traversal not allowed",
		},
		{
			name:        "blocks hidden files (.env)",
			input:       ".env",
			shouldError: true,
			errContains: "hidden files/directories not allowed",
		},
		{
			name:        "blocks hidden directories (.git)",
			input:       ".git/config",
			shouldError: true,
			errContains: "hidden files/directories not allowed",
		},
		{
			name:        "blocks .ssh directory",
			input:       ".ssh/id_rsa",
			shouldError: true,
			errContains: "hidden files/directories not allowed",
		},
		{
			name:        "allows normal paths",
			input:       "src/main.go",
			shouldError: false,
		},
		{
			name:        "allows nested paths",
			input:       "internal/adapter/verify/tools.go",
			shouldError: false,
		},
		// Cross-platform tests
		{
			name:        "blocks Windows absolute paths (C:)",
			input:       "C:\\Windows\\system.ini",
			shouldError: true,
			errContains: "absolute paths not allowed",
		},
		{
			name:        "blocks Windows path traversal",
			input:       "..\\..\\secret",
			shouldError: true,
			errContains: "path traversal not allowed",
		},
		{
			name:        "blocks UNC paths",
			input:       "\\\\server\\share\\file",
			shouldError: true,
			errContains: "not allowed",
		},
		{
			name:        "blocks mixed separator traversal",
			input:       "foo\\..\\..\\etc\\passwd",
			shouldError: true,
			errContains: "path traversal not allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tool.Execute(context.Background(), tt.input)
			if tt.shouldError {
				if err == nil {
					t.Errorf("expected error for input %q, got nil", tt.input)
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for input %q: %v", tt.input, err)
				}
			}
		})
	}
}

func TestGlobTool_Security(t *testing.T) {
	repo := &mockRepository{
		globFunc: func(pattern string) ([]string, error) {
			return []string{"file.go"}, nil
		},
	}
	tool := verify.NewGlobTool(repo)

	tests := []struct {
		name        string
		input       string
		shouldError bool
		errContains string
	}{
		{
			name:        "blocks absolute paths",
			input:       "/etc/*",
			shouldError: true,
			errContains: "absolute paths not allowed",
		},
		{
			name:        "blocks path traversal",
			input:       "../../../etc/*",
			shouldError: true,
			errContains: "path traversal not allowed",
		},
		{
			name:        "blocks .git patterns",
			input:       ".git/*",
			shouldError: true,
			errContains: "forbidden directory",
		},
		{
			name:        "blocks .env patterns",
			input:       "**/.env",
			shouldError: true,
			errContains: "forbidden directory",
		},
		{
			name:        "blocks .ssh patterns",
			input:       ".ssh/*",
			shouldError: true,
			errContains: "forbidden directory",
		},
		{
			name:        "blocks .aws patterns",
			input:       ".aws/credentials",
			shouldError: true,
			errContains: "forbidden directory",
		},
		{
			name:        "allows normal patterns",
			input:       "**/*.go",
			shouldError: false,
		},
		{
			name:        "allows specific directories",
			input:       "internal/**/*.go",
			shouldError: false,
		},
		// Cross-platform tests
		{
			name:        "blocks Windows absolute paths",
			input:       "C:\\**\\*.go",
			shouldError: true,
			errContains: "absolute paths not allowed",
		},
		{
			name:        "blocks Windows path traversal",
			input:       "..\\..\\**",
			shouldError: true,
			errContains: "path traversal not allowed",
		},
		{
			name:        "blocks UNC paths in glob",
			input:       "//server/share/*",
			shouldError: true,
			errContains: "not allowed",
		},
		{
			name:        "allows .github (not blocked by .git check)",
			input:       "src/.github/*.md",
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tool.Execute(context.Background(), tt.input)
			if tt.shouldError {
				if err == nil {
					t.Errorf("expected error for input %q, got nil", tt.input)
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for input %q: %v", tt.input, err)
				}
			}
		})
	}
}

func TestBashTool_Security(t *testing.T) {
	repo := &mockRepository{
		runCommandFunc: func(ctx context.Context, cmd string, args ...string) (usecaseverify.CommandResult, error) {
			return usecaseverify.CommandResult{Stdout: "ok", ExitCode: 0}, nil
		},
	}
	tool := verify.NewBashTool(repo)

	tests := []struct {
		name        string
		input       string
		shouldError bool
		errContains string
	}{
		// Case-insensitive checks
		{
			name:        "blocks CURL (uppercase)",
			input:       "CURL http://evil.com",
			shouldError: true,
			errContains: "not allowed",
		},
		{
			name:        "blocks Wget (mixed case)",
			input:       "Wget http://evil.com",
			shouldError: true,
			errContains: "not allowed",
		},
		// Dangerous go subcommands
		{
			name:        "blocks go test (executes code)",
			input:       "go test ./...",
			shouldError: true,
			errContains: "not allowed",
		},
		{
			name:        "blocks go run (executes code)",
			input:       "go run main.go",
			shouldError: true,
			errContains: "not allowed",
		},
		{
			name:        "blocks go generate (executes code)",
			input:       "go generate ./...",
			shouldError: true,
			errContains: "not allowed",
		},
		{
			name:        "blocks go mod (network access)",
			input:       "go mod download",
			shouldError: true,
			errContains: "not allowed",
		},
		// Shell metacharacters
		{
			name:        "blocks pipe",
			input:       "echo hello | cat",
			shouldError: true,
			errContains: "metacharacter",
		},
		{
			name:        "blocks command chaining with &&",
			input:       "ls && rm -rf /",
			shouldError: true,
			errContains: "metacharacter",
		},
		{
			name:        "blocks command substitution",
			input:       "echo $(whoami)",
			shouldError: true,
			errContains: "metacharacter",
		},
		{
			name:        "blocks redirect",
			input:       "echo secret > /tmp/file",
			shouldError: true,
			errContains: "metacharacter",
		},
		// Allowed commands
		{
			name:        "allows go build",
			input:       "go build ./...",
			shouldError: false,
		},
		{
			name:        "allows go vet",
			input:       "go vet ./...",
			shouldError: false,
		},
		{
			name:        "allows git status",
			input:       "git status",
			shouldError: false,
		},
		{
			name:        "allows git diff",
			input:       "git diff HEAD~1",
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tool.Execute(context.Background(), tt.input)
			if tt.shouldError {
				if err == nil {
					t.Errorf("expected error for input %q, got nil", tt.input)
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for input %q: %v", tt.input, err)
				}
			}
		})
	}
}

// contains checks if s contains substr (case-insensitive)
func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
