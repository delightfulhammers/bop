package review

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/delightfulhammers/bop/internal/domain"
)

// ProjectContext holds all contextual information for a code review.
type ProjectContext struct {
	// Core documentation
	Architecture string   // Contents of ARCHITECTURE.md
	README       string   // Contents of README.md
	DesignDocs   []string // Contents of design documents

	// User-provided context
	CustomInstructions string   // Instructions from --instructions flag
	CustomContextFiles []string // Contents of files from --context flag
	PlanningAnswers    string   // Answers from interactive planning phase

	// Automatically gathered context
	RelevantDocs []string // Docs related to changed files

	// Prior triage context (Issue #138)
	// Contains findings from previous reviews that have been acknowledged or disputed.
	// When populated, the LLM prompt includes this context to prevent re-raising
	// concerns that have already been addressed.
	TriagedFindings *domain.TriagedFindingContext

	// Metadata
	ChangedPaths []string // Paths of changed files
	ChangeTypes  []string // Types of changes (e.g., "auth", "database", "api")
}

// ContextConfig controls what context to gather.
type ContextConfig struct {
	// Paths to search for docs
	DesignDocsGlob string // Glob pattern for design docs (e.g., "docs/*_DESIGN.md")
}

// ContextGatherer gathers project context for code reviews.
type ContextGatherer struct {
	repoDir string
	config  ContextConfig
}

// NewContextGatherer creates a new context gatherer.
func NewContextGatherer(repoDir string) *ContextGatherer {
	return &ContextGatherer{
		repoDir: repoDir,
		config: ContextConfig{
			DesignDocsGlob: "docs/*_DESIGN.md",
		},
	}
}

// detectChangeTypes identifies the type of changes based on file paths.
// Returns a slice of change type strings (e.g., "auth", "database", "api").
func (g *ContextGatherer) detectChangeTypes(diff domain.Diff) []string {
	typeSet := make(map[string]bool)

	for _, file := range diff.Files {
		path := strings.ToLower(file.Path)

		// Auth-related
		if strings.Contains(path, "auth") || strings.Contains(path, "login") || strings.Contains(path, "session") {
			typeSet["auth"] = true
		}

		// Database-related
		if strings.Contains(path, "database") || strings.Contains(path, "migration") ||
			strings.Contains(path, "schema") || strings.Contains(path, "store") ||
			strings.Contains(path, "repository") {
			typeSet["database"] = true
		}

		// API-related
		if strings.Contains(path, "api") || strings.Contains(path, "handler") ||
			strings.Contains(path, "controller") || strings.Contains(path, "endpoint") {
			typeSet["api"] = true
		}

		// Security-related
		if strings.Contains(path, "security") || strings.Contains(path, "crypto") ||
			strings.Contains(path, "encryption") || strings.Contains(path, "redaction") {
			typeSet["security"] = true
		}

		// Configuration
		if strings.Contains(path, "config") || strings.HasSuffix(path, ".yaml") ||
			strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".toml") {
			typeSet["config"] = true
		}

		// Testing
		if strings.Contains(path, "test") || strings.HasSuffix(path, "_test.go") {
			typeSet["testing"] = true
		}

		// Documentation
		if strings.HasSuffix(path, ".md") || strings.HasSuffix(path, ".rst") ||
			strings.Contains(path, "docs/") {
			typeSet["documentation"] = true
		}

		// UI/Frontend
		if strings.Contains(path, "ui") || strings.Contains(path, "frontend") ||
			strings.Contains(path, "component") || strings.HasSuffix(path, ".tsx") ||
			strings.HasSuffix(path, ".jsx") {
			typeSet["frontend"] = true
		}
	}

	// Convert set to slice
	types := make([]string, 0, len(typeSet))
	for t := range typeSet {
		types = append(types, t)
	}

	return types
}

// loadFile reads a file from the repository.
func (g *ContextGatherer) loadFile(path string) (string, error) {
	fullPath := filepath.Join(g.repoDir, path)

	// Check if file exists and get file info
	info, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		return "", fmt.Errorf("file not found: %s", path)
	}
	if err != nil {
		return "", fmt.Errorf("failed to stat %s: %w", path, err)
	}

	// Check file size limit (1MB)
	const maxFileSize = 1 * 1024 * 1024
	if info.Size() > maxFileSize {
		return "", fmt.Errorf("file %s exceeds maximum size of 1MB (actual: %d bytes)", path, info.Size())
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", path, err)
	}

	return string(content), nil
}

// loadDesignDocs loads all design documents matching the glob pattern.
func (g *ContextGatherer) loadDesignDocs() ([]string, error) {
	pattern := filepath.Join(g.repoDir, g.config.DesignDocsGlob)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to glob design docs: %w", err)
	}

	var docs []string
	for _, match := range matches {
		content, err := os.ReadFile(match)
		if err != nil {
			continue // Skip files we can't read
		}

		relPath, _ := filepath.Rel(g.repoDir, match)
		docs = append(docs, fmt.Sprintf("=== %s ===\n%s", relPath, string(content)))
	}

	return docs, nil
}

// findRelevantDocs finds documentation related to the changed files.
// changedPaths is currently unused but reserved for future path-based matching.
func (g *ContextGatherer) findRelevantDocs(changedPaths []string, changeTypes []string) ([]string, error) {
	var relevantDocs []string

	// Map change types to relevant doc files
	docMap := map[string][]string{
		"auth":     {"docs/SECURITY.md", "docs/AUTH_DESIGN.md"},
		"database": {"docs/DATABASE_DESIGN.md"},
		"security": {"docs/SECURITY.md"},
	}

	// Track loaded paths to avoid duplicates
	loadedPaths := make(map[string]bool)

	// Load docs based on change types
	for _, changeType := range changeTypes {
		if docPaths, ok := docMap[changeType]; ok {
			for _, docPath := range docPaths {
				if loadedPaths[docPath] {
					continue // Skip already loaded
				}
				content, err := g.loadFile(docPath)
				if err == nil {
					relevantDocs = append(relevantDocs, fmt.Sprintf("=== %s ===\n%s", docPath, content))
					loadedPaths[docPath] = true
				}
			}
		}
	}

	return relevantDocs, nil
}
