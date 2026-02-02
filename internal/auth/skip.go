// Package auth provides platform authentication for bop CLI and MCP server.
package auth

import "errors"

// SkipReason represents the reason why a code review was skipped.
// These values match the platform's domain.SkipReason constants.
type SkipReason string

// Skip reason constants matching platform/internal/auth/domain/skip.go.
const (
	// SkipReasonActorNotMember indicates the GitHub actor is not a bop member.
	SkipReasonActorNotMember SkipReason = "actor_not_member"

	// SkipReasonNoActiveEntitlement indicates the actor has no active subscription.
	SkipReasonNoActiveEntitlement SkipReason = "no_active_entitlement"

	// SkipReasonSoloNamespaceViolation indicates a solo user tried to review a non-personal repo.
	SkipReasonSoloNamespaceViolation SkipReason = "solo_namespace_violation"
)

// validSkipReasons contains all valid skip reason values.
var validSkipReasons = []SkipReason{
	SkipReasonActorNotMember,
	SkipReasonNoActiveEntitlement,
	SkipReasonSoloNamespaceViolation,
}

// IsValid checks if the skip reason is valid.
func (r SkipReason) IsValid() bool {
	for _, valid := range validSkipReasons {
		if r == valid {
			return true
		}
	}
	return false
}

// String returns the string representation of the skip reason.
func (r SkipReason) String() string {
	return string(r)
}

// SkipInfo contains information about why a code review was skipped.
// This is returned by the platform when authorization passes but the actor
// doesn't have the right entitlements for the requested operation.
type SkipInfo struct {
	// Reason is the skip reason code.
	Reason SkipReason

	// Message is a human-readable message for logging.
	Message string

	// Comment is the pre-rendered markdown for a PR comment.
	Comment string
}

// UserMessage returns the human-readable message for logging.
// Returns empty string if the SkipInfo is nil.
func (s *SkipInfo) UserMessage() string {
	if s == nil {
		return ""
	}
	return s.Message
}

// PRComment returns the markdown content for posting as a PR comment.
// Returns empty string if the SkipInfo is nil.
func (s *SkipInfo) PRComment() string {
	if s == nil {
		return ""
	}
	return s.Comment
}

// ErrAuthSkipped is returned when authentication succeeds but the operation
// should be skipped due to entitlement restrictions (e.g., solo tier on org repo).
// Callers can check for this error and handle it gracefully (e.g., post PR comment).
type ErrAuthSkipped struct {
	Info *SkipInfo
}

func (e *ErrAuthSkipped) Error() string {
	if e.Info == nil {
		return "authentication skipped"
	}
	return "authentication skipped: " + e.Info.Message
}

// IsAuthSkipped returns true if the error is an ErrAuthSkipped.
func IsAuthSkipped(err error) bool {
	var skipErr *ErrAuthSkipped
	return errors.As(err, &skipErr)
}

// GetSkipInfo extracts SkipInfo from an ErrAuthSkipped error.
// Returns nil if the error is not an ErrAuthSkipped.
func GetSkipInfo(err error) *SkipInfo {
	var skipErr *ErrAuthSkipped
	if errors.As(err, &skipErr) {
		return skipErr.Info
	}
	return nil
}
