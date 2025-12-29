package repository

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bkyoung/code-reviewer/internal/usecase/verify"
)

// LocalRepository provides filesystem access rooted at a directory.
// All paths are resolved relative to the root directory.
// Path traversal attempts are blocked for security.
type LocalRepository struct {
	root string
}

// NewLocalRepository creates a new LocalRepository rooted at the given directory.
func NewLocalRepository(root string) *LocalRepository {
	return &LocalRepository{root: root}
}

// ReadFile reads the contents of a file at the given path.
// The path can be relative to the root or absolute (if within root).
func (r *LocalRepository) ReadFile(path string) ([]byte, error) {
	resolved, err := r.resolvePath(path)
	if err != nil {
		return nil, fmt.Errorf("invalid path %q: %w", path, err)
	}
	return os.ReadFile(resolved)
}

// FileExists checks if a file exists at the given path.
// Returns false for directories, permission errors, or path traversal attempts.
func (r *LocalRepository) FileExists(path string) bool {
	resolved, err := r.resolvePath(path)
	if err != nil {
		return false
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// Glob returns file paths matching the given pattern.
// Supports standard glob patterns and ** for recursive matching.
func (r *LocalRepository) Glob(pattern string) ([]string, error) {
	// Handle ** recursive pattern
	if strings.Contains(pattern, "**") {
		return r.globRecursive(pattern)
	}

	// Simple glob
	fullPattern := filepath.Join(r.root, pattern)
	matches, err := filepath.Glob(fullPattern)
	if err != nil {
		return nil, fmt.Errorf("glob pattern %q: %w", pattern, err)
	}

	// Convert to relative paths
	result := make([]string, 0, len(matches))
	for _, m := range matches {
		rel, err := filepath.Rel(r.root, m)
		if err != nil {
			continue
		}
		result = append(result, rel)
	}
	return result, nil
}

// Grep searches for a pattern in the specified files.
// If no paths are provided, searches all files in the repository.
func (r *LocalRepository) Grep(pattern string, paths ...string) ([]verify.GrepMatch, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern %q: %w", pattern, err)
	}

	var filesToSearch []string
	if len(paths) == 0 {
		// Search all files
		err := filepath.Walk(r.root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // Skip files we can't access
			}
			if !info.IsDir() && !isBinaryFile(path) {
				rel, err := filepath.Rel(r.root, path)
				if err == nil {
					filesToSearch = append(filesToSearch, rel)
				}
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walking directory: %w", err)
		}
	} else {
		filesToSearch = paths
	}

	var matches []verify.GrepMatch
	for _, path := range filesToSearch {
		fileMatches, err := r.grepFile(re, path)
		if err != nil {
			continue // Skip files we can't read
		}
		matches = append(matches, fileMatches...)
	}

	return matches, nil
}

// RunCommand executes a command in the repository directory.
//
// SECURITY: This method allows arbitrary command execution within the repository.
// Callers are responsible for:
// - Validating/sanitizing command and arguments
// - Implementing allowlists for permitted commands if needed
// - Enforcing appropriate timeouts via context
func (r *LocalRepository) RunCommand(ctx context.Context, cmd string, args ...string) (verify.CommandResult, error) {
	command := exec.CommandContext(ctx, cmd, args...)
	command.Dir = r.root

	var stdout, stderr strings.Builder
	command.Stdout = &stdout
	command.Stderr = &stderr

	err := command.Run()

	result := verify.CommandResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			return result, nil
		}
		return result, fmt.Errorf("running command %q: %w", cmd, err)
	}

	return result, nil
}

// resolvePath resolves a path and validates it's within the repository root.
// It follows symlinks to prevent bypassing the root directory check.
// Returns the real (symlink-resolved) path to prevent TOCTOU attacks.
func (r *LocalRepository) resolvePath(path string) (string, error) {
	var resolved string

	if filepath.IsAbs(path) {
		resolved = path
	} else {
		resolved = filepath.Join(r.root, path)
	}

	// Clean the path to resolve any .. components
	resolved = filepath.Clean(resolved)

	// Get the real root path (following symlinks)
	realRoot, err := filepath.EvalSymlinks(r.root)
	if err != nil {
		// If root doesn't exist, use cleaned path
		realRoot = filepath.Clean(r.root)
	}

	// Resolve symlinks to get the real path
	// This prevents symlink-based path traversal attacks
	realPath, err := filepath.EvalSymlinks(resolved)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("resolving symlinks: %w", err)
		}
		// File doesn't exist - validate the cleaned path instead
		// Use filepath.Rel to properly check path hierarchy
		rel, relErr := filepath.Rel(realRoot, resolved)
		if relErr != nil || strings.HasPrefix(rel, "..") {
			return "", fmt.Errorf("path traversal detected")
		}
		return resolved, nil
	}

	// Use filepath.Rel to check if realPath is under realRoot
	// This correctly handles cases like /data vs /data-secret
	rel, err := filepath.Rel(realRoot, realPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path traversal detected")
	}

	// Return the real path to prevent TOCTOU attacks
	return realPath, nil
}

// globRecursive handles ** patterns for recursive directory matching.
func (r *LocalRepository) globRecursive(pattern string) ([]string, error) {
	// Split pattern by **
	parts := strings.Split(pattern, "**")
	if len(parts) != 2 {
		return nil, fmt.Errorf("only one ** is supported in pattern")
	}

	prefix := strings.TrimSuffix(parts[0], string(filepath.Separator))
	suffix := strings.TrimPrefix(parts[1], string(filepath.Separator))

	var matches []string
	searchRoot := r.root
	if prefix != "" {
		searchRoot = filepath.Join(r.root, prefix)
	}

	err := filepath.Walk(searchRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip inaccessible paths
		}
		if info.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(r.root, path)
		if err != nil {
			return nil
		}

		// Check if file matches suffix pattern
		if suffix == "" {
			matches = append(matches, rel)
			return nil
		}

		matched, err := filepath.Match(suffix, filepath.Base(path))
		if err == nil && matched {
			matches = append(matches, rel)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walking directory: %w", err)
	}

	return matches, nil
}

// grepFile searches for a pattern in a single file.
func (r *LocalRepository) grepFile(re *regexp.Regexp, path string) ([]verify.GrepMatch, error) {
	resolved, err := r.resolvePath(path)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(resolved)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	var matches []verify.GrepMatch
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if re.MatchString(line) {
			matches = append(matches, verify.GrepMatch{
				File:    path,
				Line:    lineNum,
				Content: line,
			})
		}
	}

	return matches, scanner.Err()
}

// isBinaryFile checks if a file is likely binary based on its extension.
func isBinaryFile(path string) bool {
	binaryExtensions := map[string]bool{
		".exe": true, ".dll": true, ".so": true, ".dylib": true,
		".zip": true, ".tar": true, ".gz": true, ".rar": true,
		".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".bmp": true,
		".pdf": true, ".doc": true, ".docx": true,
		".o": true, ".a": true, ".obj": true,
	}
	ext := strings.ToLower(filepath.Ext(path))
	return binaryExtensions[ext]
}
