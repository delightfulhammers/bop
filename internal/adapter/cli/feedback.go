package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/delightfulhammers/bop/internal/adapter/feedback"
	platformfeedback "github.com/delightfulhammers/platform/contracts/feedback"
)

// FeedbackClient defines the interface for feedback operations.
type FeedbackClient interface {
	Submit(ctx context.Context, req feedback.SubmitRequest) (*feedback.SubmitResponse, error)
	List(ctx context.Context) (*feedback.ListResponse, error)
	Get(ctx context.Context, id uuid.UUID) (*platformfeedback.Feedback, error)
}

// NewFeedbackCommand creates the feedback command group.
func NewFeedbackCommand(client FeedbackClient, authDeps AuthDependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "feedback",
		Short: "Submit and view product feedback",
		Long: `Submit feedback about bop to help us improve.

Use 'bop feedback submit' to submit new feedback.
Use 'bop feedback list' to view your submitted feedback.
Use 'bop feedback show <id>' to view a specific feedback item.`,
	}

	cmd.AddCommand(newFeedbackSubmitCommand(client, authDeps))
	cmd.AddCommand(newFeedbackListCommand(client, authDeps))
	cmd.AddCommand(newFeedbackShowCommand(client, authDeps))

	return cmd
}

func newFeedbackSubmitCommand(client FeedbackClient, authDeps AuthDependencies) *cobra.Command {
	var category string
	var title string
	var description string

	cmd := &cobra.Command{
		Use:   "submit",
		Short: "Submit new feedback",
		Long: `Submit feedback about bop to help us improve.

Categories:
  bug        - Report a bug or issue
  feature    - Request a new feature
  usability  - Report usability issues
  other      - General feedback

Example:
  bop feedback submit --category bug --title "Review fails on large files" --description "When reviewing files over 10MB..."`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Require auth in platform mode
			if _, err := authDeps.RequireAuth(); err != nil {
				return err
			}

			// Validate category
			cat := platformfeedback.Category(strings.ToUpper(category))
			if !cat.IsValid() {
				return fmt.Errorf("invalid category %q - must be one of: bug, feature, usability, other", category)
			}

			// Validate required fields
			if title == "" {
				return fmt.Errorf("--title is required")
			}
			if description == "" {
				return fmt.Errorf("--description is required")
			}

			resp, err := client.Submit(cmd.Context(), feedback.SubmitRequest{
				Category:    cat,
				Title:       title,
				Description: description,
				ClientType:  platformfeedback.ClientTypeCLI,
			})
			if err != nil {
				return fmt.Errorf("submit feedback: %w", err)
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Feedback submitted successfully!\n")
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  ID: %s\n", resp.ID)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Status: %s\n", resp.Status)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nThank you for your feedback!\n")

			return nil
		},
	}

	cmd.Flags().StringVarP(&category, "category", "c", "", "Feedback category: bug, feature, usability, other (required)")
	cmd.Flags().StringVarP(&title, "title", "t", "", "Brief summary of the feedback (required)")
	cmd.Flags().StringVarP(&description, "description", "d", "", "Detailed description of the feedback (required)")
	_ = cmd.MarkFlagRequired("category")
	_ = cmd.MarkFlagRequired("title")
	_ = cmd.MarkFlagRequired("description")

	return cmd
}

func newFeedbackListCommand(client FeedbackClient, authDeps AuthDependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List your submitted feedback",
		Long:  `View all feedback you have submitted.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Require auth in platform mode
			if _, err := authDeps.RequireAuth(); err != nil {
				return err
			}

			resp, err := client.List(cmd.Context())
			if err != nil {
				return fmt.Errorf("list feedback: %w", err)
			}

			if len(resp.Feedback) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No feedback submitted yet.")
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "\nUse 'bop feedback submit' to submit your first feedback.")
				return nil
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Your feedback (%d total):\n\n", resp.Total)

			for _, f := range resp.Feedback {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", f.ID)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "    Category: %s\n", f.Category)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "    Title:    %s\n", f.Title)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "    Status:   %s\n", f.Status)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "    Created:  %s\n\n", f.CreatedAt.Format("2006-01-02 15:04"))
			}

			if resp.HasMore {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "(More feedback available)")
			}

			return nil
		},
	}

	return cmd
}

func newFeedbackShowCommand(client FeedbackClient, authDeps AuthDependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show details of a specific feedback item",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Require auth in platform mode
			if _, err := authDeps.RequireAuth(); err != nil {
				return err
			}

			id, err := uuid.Parse(args[0])
			if err != nil {
				return fmt.Errorf("invalid feedback ID: %w", err)
			}

			f, err := client.Get(cmd.Context(), id)
			if err != nil {
				return fmt.Errorf("get feedback: %w", err)
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Feedback: %s\n\n", f.ID)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Category:    %s\n", f.Category)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Title:       %s\n", f.Title)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Status:      %s\n", f.Status)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Created:     %s\n", f.CreatedAt.Format("2006-01-02 15:04:05"))
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Updated:     %s\n", f.UpdatedAt.Format("2006-01-02 15:04:05"))
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nDescription:\n%s\n", f.Description)

			if f.AdminNotes != "" {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nAdmin Notes:\n%s\n", f.AdminNotes)
			}

			return nil
		},
	}

	return cmd
}
