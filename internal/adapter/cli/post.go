package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bkyoung/code-reviewer/internal/domain"
	"github.com/bkyoung/code-reviewer/internal/usecase/post"
)

// FindingsPoster defines the dependency for posting findings.
type FindingsPoster interface {
	PostFindings(ctx context.Context, req post.Request) (*post.Result, error)
}

// PostCommand creates the 'post' command for posting findings to a GitHub PR.
func PostCommand(poster FindingsPoster) *cobra.Command {
	var owner string
	var repo string
	var prNumber int
	var dryRun bool
	var reviewAction string

	cmd := &cobra.Command{
		Use:   "post <json-file>",
		Short: "Post findings from a JSON file to a GitHub PR",
		Long: `Reads findings from a previously saved review output and posts
them to a GitHub PR. This enables a review-then-post workflow where
you can inspect and optionally edit findings before posting.

The JSON file can be:
  - Full review output from 'cr review branch --format json'
  - Raw array of findings for maximum flexibility

Examples:
  cr post ./review/findings.json --owner bkyoung --repo code-reviewer --pr 217
  cr post ./review.json --owner owner --repo repo --pr 123 --dry-run
  cr post ./review.json --owner owner --repo repo --pr 123 --review-action COMMENT`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate dependency (fail fast if not properly wired)
			if poster == nil {
				return fmt.Errorf("post command is not available: GitHub integration not configured (missing GITHUB_TOKEN)")
			}

			filePath := args[0]
			ctx := cmd.Context()

			// Read and parse the JSON file
			findings, err := parseFindings(filePath)
			if err != nil {
				return err
			}

			// Prepare review action override
			var actionPtr *string
			if reviewAction != "" {
				normalized := strings.ToUpper(reviewAction)
				if normalized != "COMMENT" && normalized != "REQUEST_CHANGES" && normalized != "APPROVE" {
					return fmt.Errorf("invalid review action %q: must be COMMENT, REQUEST_CHANGES, or APPROVE", reviewAction)
				}
				actionPtr = &normalized
			}

			// Build the request
			req := post.Request{
				Owner:        owner,
				Repo:         repo,
				PRNumber:     prNumber,
				Findings:     findings,
				DryRun:       dryRun,
				ReviewAction: actionPtr,
			}

			// Post findings
			result, err := poster.PostFindings(ctx, req)
			if err != nil {
				return err
			}

			// Output result
			if result.DryRun {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Dry run: would post %d finding(s) to %s/%s#%d\n",
					result.WouldPost, owner, repo, prNumber)
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Posted %d finding(s) to %s/%s#%d\n",
					result.Posted, owner, repo, prNumber)
				if result.Skipped > 0 {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Skipped: %d (not in diff)\n", result.Skipped)
				}
				if result.Duplicates > 0 {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Duplicates: %d\n", result.Duplicates)
				}
				if result.ReviewURL != "" {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Review: %s\n", result.ReviewURL)
				}
			}

			return nil
		},
	}

	// Required flags
	cmd.Flags().StringVar(&owner, "owner", "", "GitHub repository owner (required)")
	cmd.Flags().StringVar(&repo, "repo", "", "GitHub repository name (required)")
	cmd.Flags().IntVar(&prNumber, "pr", 0, "Pull request number (required)")
	_ = cmd.MarkFlagRequired("owner")
	_ = cmd.MarkFlagRequired("repo")
	_ = cmd.MarkFlagRequired("pr")

	// Optional flags
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be posted without posting")
	cmd.Flags().StringVar(&reviewAction, "review-action", "", "Review action: COMMENT, REQUEST_CHANGES, or APPROVE (default: auto based on severity)")

	return cmd
}

// jsonOutputWithRaw uses json.RawMessage to detect presence of "findings" field.
type jsonOutputWithRaw struct {
	Findings json.RawMessage `json:"findings"`
}

// parseFindings reads a JSON file and extracts findings.
// Supports both full review output (with "findings" field) and raw findings array.
func parseFindings(filePath string) ([]domain.Finding, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	// Detect JSON shape by first byte
	trimmed := trimLeadingWhitespace(data)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("failed to parse findings from %s: file is empty", filePath)
	}

	switch trimmed[0] {
	case '[':
		// Raw array of findings
		var findings []domain.Finding
		if err := json.Unmarshal(data, &findings); err != nil {
			return nil, fmt.Errorf("failed to parse findings from %s: %w", filePath, err)
		}
		return findings, nil

	case '{':
		// JSON object - check for "findings" field
		var raw jsonOutputWithRaw
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("failed to parse findings from %s: %w", filePath, err)
		}

		// Check if "findings" field exists
		if raw.Findings == nil {
			return nil, fmt.Errorf("failed to parse findings from %s: JSON object missing 'findings' field", filePath)
		}

		// Parse the findings array
		var findings []domain.Finding
		if err := json.Unmarshal(raw.Findings, &findings); err != nil {
			return nil, fmt.Errorf("failed to parse findings from %s: invalid 'findings' array: %w", filePath, err)
		}
		return findings, nil // May be empty, that's OK

	default:
		return nil, fmt.Errorf("failed to parse findings from %s: expected JSON array or object, got unexpected character %q", filePath, trimmed[0])
	}
}

// trimLeadingWhitespace returns data with leading whitespace removed.
func trimLeadingWhitespace(data []byte) []byte {
	for i, b := range data {
		if b != ' ' && b != '\t' && b != '\n' && b != '\r' {
			return data[i:]
		}
	}
	return nil
}
