package verify

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/delightfulhammers/bop/internal/usecase/verify"
)

// MaxToolOutputLength is the maximum length of tool output before truncation.
// This prevents runaway memory usage from large files or command output.
const MaxToolOutputLength = 50000

// Tool defines the interface for verification agent tools.
// Tools provide capabilities for the agent to investigate candidate findings.
type Tool interface {
	// Name returns the tool identifier used in prompts and logs.
	Name() string

	// Description returns a human-readable description for the agent prompt.
	Description() string

	// Execute runs the tool with the given input and returns the result.
	// The context allows for cancellation and timeout.
	Execute(ctx context.Context, input string) (string, error)
}

// NewToolRegistry creates all verification tools from a repository.
func NewToolRegistry(repo verify.Repository) []Tool {
	return []Tool{
		NewReadFileTool(repo),
		NewGrepTool(repo),
		NewGlobTool(repo),
		NewBashTool(repo),
	}
}

// ReadFileTool reads file contents from the repository.
type ReadFileTool struct {
	repo verify.Repository
}

// NewReadFileTool creates a new read file tool.
func NewReadFileTool(repo verify.Repository) *ReadFileTool {
	return &ReadFileTool{repo: repo}
}

// Name returns the tool name.
func (t *ReadFileTool) Name() string {
	return "read_file"
}

// Description returns the tool description.
func (t *ReadFileTool) Description() string {
	return "Read the contents of a file. Input: file path (e.g., 'src/main.go')"
}

// Execute reads the file at the given path.
func (t *ReadFileTool) Execute(ctx context.Context, input string) (string, error) {
	filePath := strings.TrimSpace(input)
	if filePath == "" {
		return "", fmt.Errorf("file path required")
	}

	// Validate path to prevent traversal attacks
	if err := validatePath(filePath); err != nil {
		return "", err
	}

	content, err := t.repo.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("reading file %s: %w", filePath, err)
	}

	result := string(content)
	return truncateOutput(result), nil
}

// validatePath checks that a path is safe (no traversal, no absolute paths).
// Works across platforms by normalizing separators and using filepath package.
func validatePath(filePath string) error {
	// Normalize backslashes to forward slashes for consistent checking
	normalized := strings.ReplaceAll(filePath, "\\", "/")

	// Block absolute paths (Unix-style and Windows-style)
	if filepath.IsAbs(filePath) || strings.HasPrefix(normalized, "/") {
		return fmt.Errorf("absolute paths not allowed: %s", filePath)
	}

	// Block Windows drive letters (C:, D:, etc.)
	// Must be letter followed by colon
	if len(normalized) >= 2 && normalized[1] == ':' {
		first := normalized[0]
		if (first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z') {
			return fmt.Errorf("absolute paths not allowed: %s", filePath)
		}
	}

	// Block UNC paths (\\server\share or //server/share)
	if strings.HasPrefix(normalized, "//") {
		return fmt.Errorf("UNC paths not allowed: %s", filePath)
	}

	// Check for path traversal BEFORE cleaning to catch all variants
	// This includes: .., ../, ..\, foo/../bar, etc.
	if containsTraversal(normalized) {
		return fmt.Errorf("path traversal not allowed: %s", filePath)
	}

	// Clean the path to resolve . components
	cleaned := filepath.Clean(filePath)

	// Double-check after cleaning (path.Clean may produce different results)
	cleanedNorm := strings.ReplaceAll(cleaned, "\\", "/")
	if containsTraversal(cleanedNorm) {
		return fmt.Errorf("path traversal not allowed: %s", filePath)
	}

	// Check for hidden files/directories (starting with .)
	// Use the normalized path with forward slashes for splitting
	parts := strings.Split(cleanedNorm, "/")
	for _, part := range parts {
		if part == "" {
			continue
		}
		// Block hidden files/directories but allow "." as current dir reference
		if strings.HasPrefix(part, ".") && part != "." {
			return fmt.Errorf("hidden files/directories not allowed: %s", filePath)
		}
	}

	return nil
}

// containsTraversal checks if a normalized path contains directory traversal.
func containsTraversal(normalizedPath string) bool {
	parts := strings.Split(normalizedPath, "/")
	for _, part := range parts {
		if part == ".." {
			return true
		}
	}
	return false
}

// validateGlobPattern checks that a glob pattern is safe.
func validateGlobPattern(pattern string) error {
	// Normalize backslashes to forward slashes
	normalized := strings.ReplaceAll(pattern, "\\", "/")
	normalizedLower := strings.ToLower(normalized)

	// Block absolute paths (Unix and Windows style)
	if strings.HasPrefix(normalized, "/") {
		return fmt.Errorf("absolute paths not allowed in glob: %s", pattern)
	}

	// Block Windows drive letters (must be letter followed by colon)
	if len(normalized) >= 2 && normalized[1] == ':' {
		first := normalized[0]
		if (first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z') {
			return fmt.Errorf("absolute paths not allowed in glob: %s", pattern)
		}
	}

	// Block UNC paths
	if strings.HasPrefix(normalized, "//") {
		return fmt.Errorf("UNC paths not allowed in glob: %s", pattern)
	}

	// Block patterns that contain path traversal
	if containsTraversal(normalized) {
		return fmt.Errorf("path traversal not allowed in glob: %s", pattern)
	}

	// Block patterns explicitly targeting hidden/sensitive directories
	// Use case-insensitive matching and check path segments to avoid false positives
	// (e.g., don't block "src/.github" just because it contains ".git" substring)
	forbiddenDirs := []string{".git", ".env", ".ssh", ".aws", ".config", ".secret"}
	parts := strings.Split(normalizedLower, "/")
	for _, part := range parts {
		// Skip glob wildcards and empty parts
		if part == "" || part == "*" || part == "**" {
			continue
		}
		for _, forbidden := range forbiddenDirs {
			// Check exact match of the segment
			if part == forbidden {
				return fmt.Errorf("pattern targets forbidden directory: %s", forbidden)
			}
		}
	}

	return nil
}

// GrepTool searches for patterns in the repository.
type GrepTool struct {
	repo verify.Repository
}

// NewGrepTool creates a new grep tool.
func NewGrepTool(repo verify.Repository) *GrepTool {
	return &GrepTool{repo: repo}
}

// Name returns the tool name.
func (t *GrepTool) Name() string {
	return "grep"
}

// Description returns the tool description.
func (t *GrepTool) Description() string {
	return "Search for a pattern in the codebase. Input: search pattern (regex supported)"
}

// Execute searches for the pattern in the repository.
func (t *GrepTool) Execute(ctx context.Context, input string) (string, error) {
	pattern := strings.TrimSpace(input)
	if pattern == "" {
		return "", fmt.Errorf("search pattern required")
	}

	matches, err := t.repo.Grep(pattern)
	if err != nil {
		return "", fmt.Errorf("grep %s: %w", pattern, err)
	}

	if len(matches) == 0 {
		return "No matches found", nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Found %d matches:\n", len(matches))
	for _, m := range matches {
		fmt.Fprintf(&sb, "%s:%d: %s\n", m.File, m.Line, m.Content)
	}

	return truncateOutput(sb.String()), nil
}

// GlobTool finds files matching a pattern.
type GlobTool struct {
	repo verify.Repository
}

// NewGlobTool creates a new glob tool.
func NewGlobTool(repo verify.Repository) *GlobTool {
	return &GlobTool{repo: repo}
}

// Name returns the tool name.
func (t *GlobTool) Name() string {
	return "glob"
}

// Description returns the tool description.
func (t *GlobTool) Description() string {
	return "Find files matching a pattern. Input: glob pattern (e.g., '**/*.go', 'internal/**/test_*.go')"
}

// Execute finds files matching the pattern.
func (t *GlobTool) Execute(ctx context.Context, input string) (string, error) {
	pattern := strings.TrimSpace(input)
	if pattern == "" {
		return "", fmt.Errorf("glob pattern required")
	}

	// Validate pattern to prevent access to sensitive files
	if err := validateGlobPattern(pattern); err != nil {
		return "", err
	}

	files, err := t.repo.Glob(pattern)
	if err != nil {
		return "", fmt.Errorf("glob %s: %w", pattern, err)
	}

	if len(files) == 0 {
		return "No files found matching pattern", nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Found %d files:\n", len(files))
	for _, f := range files {
		sb.WriteString(f + "\n")
	}

	return truncateOutput(sb.String()), nil
}

// BashTool runs safe commands in the repository.
type BashTool struct {
	repo verify.Repository
}

// NewBashTool creates a new bash tool.
func NewBashTool(repo verify.Repository) *BashTool {
	return &BashTool{repo: repo}
}

// Name returns the tool name.
func (t *BashTool) Name() string {
	return "bash"
}

// Description returns the tool description.
func (t *BashTool) Description() string {
	return "Run a safe command (go build, go vet, go test, git diff, etc.). Input: command and arguments"
}

// safeCommands defines which commands are allowed and their safe subcommands.
// Commands with nil subcommand lists are allowed with any arguments.
// Commands with non-nil subcommand lists are only allowed with those specific subcommands.
//
// SECURITY: We intentionally exclude commands that can execute arbitrary code:
// - "go test" executes test code and init() functions
// - "go run" executes arbitrary Go code
// - "go generate" executes arbitrary commands
// - "go mod download/tidy" can access the network
var safeCommands = map[string][]string{
	// Go commands - only read-only static analysis operations
	// Excluded: test (executes code), run, generate, mod (network access)
	"go": {"build", "vet", "list", "version", "env"},
	// Git commands - only read-only operations
	"git": {"status", "log", "show", "diff", "branch", "rev-parse", "describe", "ls-files"},
	// Read-only utilities
	"echo": nil, // Any args OK for echo
	"head": nil, // Any args OK
	"tail": nil, // Any args OK
	"wc":   nil, // Any args OK
	"ls":   nil, // Any args OK
}

// dangerousCommands are command names that should never be allowed.
// These are checked as complete tokens (not substrings) to avoid false positives.
var dangerousCommands = map[string]bool{
	// File deletion/modification
	"rm": true, "rmdir": true, "mv": true, "dd": true,
	// Network access
	"curl": true, "wget": true, "nc": true, "netcat": true,
	"ssh": true, "scp": true, "rsync": true, "ftp": true,
	// Privilege escalation
	"chmod": true, "chown": true, "sudo": true, "su": true,
	// Code execution
	"eval": true, "exec": true, "xargs": true, "env": true,
	// Shell spawning
	"sh": true, "bash": true, "zsh": true, "ksh": true, "csh": true,
	"python": true, "python3": true, "python2": true,
	"ruby": true, "perl": true, "node": true, "php": true,
}

// shellMetacharacters are patterns that indicate shell features that could bypass restrictions.
// These use substring matching because they're actual characters in the input.
var shellMetacharacters = []string{
	">",  // Redirect output
	"<",  // Redirect input
	"|",  // Pipe
	";",  // Command chaining
	"&&", // Conditional chaining
	"||", // Conditional chaining
	"`",  // Command substitution (backtick)
	"$(", // Command substitution
	"${", // Variable expansion
	"\n", // Newline (could inject commands)
}

// checkDangerousTokens checks if any token in the input is a dangerous command.
// This uses token-based matching to avoid false positives.
func checkDangerousTokens(input string) error {
	// Split on whitespace to get tokens
	tokens := strings.Fields(input)

	for _, token := range tokens {
		// Normalize to lowercase for case-insensitive matching
		tokenLower := strings.ToLower(token)

		// Check if this token is a dangerous command
		if dangerousCommands[tokenLower] {
			return fmt.Errorf("command %q is not allowed", token)
		}
	}

	return nil
}

// Execute runs the command if it's in the allowlist.
func (t *BashTool) Execute(ctx context.Context, input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("command required")
	}

	// Check for shell metacharacters (these need substring matching)
	for _, meta := range shellMetacharacters {
		if strings.Contains(input, meta) {
			return "", fmt.Errorf("command contains forbidden shell metacharacter: %q", meta)
		}
	}

	// Check for dangerous commands as tokens (not substrings)
	// This prevents false positives like blocking "format" because it contains "rm"
	if err := checkDangerousTokens(input); err != nil {
		return "", err
	}

	// Parse command
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return "", fmt.Errorf("empty command")
	}

	cmd := parts[0]
	args := parts[1:]

	// Check allowlist
	allowedSubcmds, cmdAllowed := safeCommands[cmd]
	if !cmdAllowed {
		return "", fmt.Errorf("command %q not in allowlist", cmd)
	}

	// If subcommands are restricted, verify the first argument is allowed
	if allowedSubcmds != nil {
		if len(args) == 0 {
			return "", fmt.Errorf("command %q requires a subcommand", cmd)
		}
		subcmd := args[0]
		allowed := false
		for _, s := range allowedSubcmds {
			if s == subcmd {
				allowed = true
				break
			}
		}
		if !allowed {
			return "", fmt.Errorf("subcommand %q not allowed for %q (allowed: %v)", subcmd, cmd, allowedSubcmds)
		}
	}

	result, err := t.repo.RunCommand(ctx, cmd, args...)
	if err != nil {
		return "", fmt.Errorf("running command: %w", err)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Exit code: %d\n", result.ExitCode)

	if result.Stdout != "" {
		sb.WriteString("Stdout:\n")
		sb.WriteString(result.Stdout)
		sb.WriteString("\n")
	}

	if result.Stderr != "" {
		sb.WriteString("Stderr:\n")
		sb.WriteString(result.Stderr)
		sb.WriteString("\n")
	}

	return truncateOutput(sb.String()), nil
}

// truncateOutput truncates output that exceeds MaxToolOutputLength.
func truncateOutput(s string) string {
	if len(s) <= MaxToolOutputLength {
		return s
	}
	return s[:MaxToolOutputLength] + "\n... [output truncated]"
}
