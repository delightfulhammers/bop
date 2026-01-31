package auth

import "testing"

func TestSkipReason_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		reason SkipReason
		want   bool
	}{
		{
			name:   "actor_not_member is valid",
			reason: SkipReasonActorNotMember,
			want:   true,
		},
		{
			name:   "no_active_entitlement is valid",
			reason: SkipReasonNoActiveEntitlement,
			want:   true,
		},
		{
			name:   "solo_namespace_violation is valid",
			reason: SkipReasonSoloNamespaceViolation,
			want:   true,
		},
		{
			name:   "empty string is invalid",
			reason: SkipReason(""),
			want:   false,
		},
		{
			name:   "unknown reason is invalid",
			reason: SkipReason("unknown_reason"),
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.reason.IsValid(); got != tt.want {
				t.Errorf("SkipReason(%q).IsValid() = %v, want %v", tt.reason, got, tt.want)
			}
		})
	}
}

func TestSkipInfo_UserMessage(t *testing.T) {
	tests := []struct {
		name string
		skip *SkipInfo
		want string
	}{
		{
			name: "actor not member",
			skip: &SkipInfo{
				Reason:  SkipReasonActorNotMember,
				Message: "actor testuser is not a bop member",
			},
			want: "actor testuser is not a bop member",
		},
		{
			name: "no active entitlement",
			skip: &SkipInfo{
				Reason:  SkipReasonNoActiveEntitlement,
				Message: "actor testuser has no active entitlement",
			},
			want: "actor testuser has no active entitlement",
		},
		{
			name: "solo namespace violation",
			skip: &SkipInfo{
				Reason:  SkipReasonSoloNamespaceViolation,
				Message: "actor testuser has solo plan but repo owner is orgname",
			},
			want: "actor testuser has solo plan but repo owner is orgname",
		},
		{
			name: "nil skip info",
			skip: nil,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.skip.UserMessage(); got != tt.want {
				t.Errorf("SkipInfo.UserMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSkipInfo_CommentMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		skip     *SkipInfo
		contains []string // strings that should be in the comment
	}{
		{
			name: "actor not member has proper markdown",
			skip: &SkipInfo{
				Reason:  SkipReasonActorNotMember,
				Comment: "## Code review skipped\n\nActor @testuser is not a bop member.",
			},
			contains: []string{"Code review skipped", "@testuser", "not a bop member"},
		},
		{
			name: "solo namespace violation has upgrade link",
			skip: &SkipInfo{
				Reason:  SkipReasonSoloNamespaceViolation,
				Comment: "## Code review skipped\n\nActor @testuser has a Solo plan.\n\n[Upgrade to Pro](https://delightfulhammers.com/bop/upgrade)",
			},
			contains: []string{"Code review skipped", "Solo plan", "Upgrade to Pro"},
		},
		{
			name: "nil skip info returns empty",
			skip: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comment := tt.skip.PRComment()
			if tt.skip == nil {
				if comment != "" {
					t.Errorf("nil SkipInfo.PRComment() = %q, want empty", comment)
				}
				return
			}
			for _, substr := range tt.contains {
				if !contains(comment, substr) {
					t.Errorf("SkipInfo.PRComment() = %q, missing expected substring %q", comment, substr)
				}
			}
		})
	}
}

// contains is a simple helper since strings.Contains is what we need
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
