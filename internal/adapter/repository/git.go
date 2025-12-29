package repository

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bkyoung/code-reviewer/internal/usecase/verify"
)

// GitRepository extends LocalRepository with git-awareness.
// It respects .gitignore patterns when globbing and grepping.
// ReadFile and FileExists work on all files regardless of .gitignore.
type GitRepository struct {
	*LocalRepository
	ignorePatterns []gitignorePattern
	isGitRepo      bool
}

// gitignorePattern represents a single .gitignore pattern.
type gitignorePattern struct {
	pattern  string
	negation bool // true if pattern starts with !
	dirOnly  bool // true if pattern ends with /
}

// NewGitRepository creates a git-aware repository.
// If the directory is not a git repository, it behaves like LocalRepository.
func NewGitRepository(root string) *GitRepository {
	repo := &GitRepository{
		LocalRepository: NewLocalRepository(root),
	}

	// Check if this is a git repository
	gitDir := filepath.Join(root, ".git")
	if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
		repo.isGitRepo = true
		repo.loadGitignore(root)
	}

	return repo
}

// Glob returns file paths matching the pattern, excluding .gitignore patterns.
func (r *GitRepository) Glob(pattern string) ([]string, error) {
	matches, err := r.LocalRepository.Glob(pattern)
	if err != nil {
		return nil, err
	}

	if !r.isGitRepo {
		return matches, nil
	}

	// Filter out ignored files
	filtered := make([]string, 0, len(matches))
	for _, m := range matches {
		if !r.isIgnored(m) {
			filtered = append(filtered, m)
		}
	}
	return filtered, nil
}

// Grep searches for a pattern, excluding files matching .gitignore.
func (r *GitRepository) Grep(pattern string, paths ...string) ([]verify.GrepMatch, error) {
	if !r.isGitRepo || len(paths) > 0 {
		// If not a git repo or specific paths given, use LocalRepository
		return r.LocalRepository.Grep(pattern, paths...)
	}

	// Get all non-ignored files
	files, err := r.getTrackedFiles()
	if err != nil {
		return nil, err
	}

	// Grep in non-ignored files
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	var matches []verify.GrepMatch
	for _, path := range files {
		fileMatches, err := r.grepFile(re, path)
		if err != nil {
			continue
		}
		matches = append(matches, fileMatches...)
	}

	return matches, nil
}

// loadGitignore reads and parses the .gitignore file.
func (r *GitRepository) loadGitignore(root string) {
	gitignorePath := filepath.Join(root, ".gitignore")
	file, err := os.Open(gitignorePath)
	if err != nil {
		return // No .gitignore file
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		pattern := gitignorePattern{pattern: line}

		// Check for negation
		if strings.HasPrefix(line, "!") {
			pattern.negation = true
			pattern.pattern = line[1:]
		}

		// Check for directory-only pattern
		if strings.HasSuffix(pattern.pattern, "/") {
			pattern.dirOnly = true
			pattern.pattern = strings.TrimSuffix(pattern.pattern, "/")
		}

		r.ignorePatterns = append(r.ignorePatterns, pattern)
	}

	// Always ignore .git directory
	r.ignorePatterns = append(r.ignorePatterns, gitignorePattern{
		pattern: ".git",
		dirOnly: true,
	})
}

// isIgnored checks if a path matches any .gitignore pattern.
func (r *GitRepository) isIgnored(path string) bool {
	if len(r.ignorePatterns) == 0 {
		return false
	}

	// Normalize path separators
	path = filepath.ToSlash(path)
	parts := strings.Split(path, "/")

	ignored := false
	for _, pattern := range r.ignorePatterns {
		if r.matchesPattern(path, parts, pattern) {
			ignored = !pattern.negation
		}
	}
	return ignored
}

// matchesPattern checks if a path matches a gitignore pattern.
func (r *GitRepository) matchesPattern(path string, parts []string, pattern gitignorePattern) bool {
	p := pattern.pattern

	// Check if pattern matches any component of the path
	for _, part := range parts {
		if matched, _ := filepath.Match(p, part); matched {
			return true
		}
	}

	// Check if pattern matches the full path or file name
	if matched, _ := filepath.Match(p, filepath.Base(path)); matched {
		return true
	}
	if matched, _ := filepath.Match(p, path); matched {
		return true
	}

	// Handle patterns with wildcards
	if strings.Contains(p, "*") {
		if matched, _ := filepath.Match(p, filepath.Base(path)); matched {
			return true
		}
	}

	// Handle directory patterns (pattern matches path prefix)
	if pattern.dirOnly || !strings.Contains(p, ".") {
		if strings.HasPrefix(path, p+"/") {
			return true
		}
		// Check each component
		for _, part := range parts {
			if part == p {
				return true
			}
		}
	}

	return false
}

// getTrackedFiles returns all files not matching .gitignore patterns.
func (r *GitRepository) getTrackedFiles() ([]string, error) {
	var files []string

	err := filepath.Walk(r.root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip inaccessible paths
		}

		rel, err := filepath.Rel(r.root, path)
		if err != nil {
			return nil
		}

		// Skip directories that are ignored (prune)
		if info.IsDir() {
			if r.isIgnored(rel) {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip ignored files and binary files
		if r.isIgnored(rel) || isBinaryFile(path) {
			return nil
		}

		files = append(files, rel)
		return nil
	})

	return files, err
}

// grepFile searches for a pattern in a single file.
func (r *GitRepository) grepFile(re *regexp.Regexp, path string) ([]verify.GrepMatch, error) {
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
