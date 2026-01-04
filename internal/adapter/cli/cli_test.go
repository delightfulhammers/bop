package cli_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/delightfulhammers/bop/internal/adapter/cli"
	"github.com/delightfulhammers/bop/internal/usecase/review"
)

type branchStub struct {
	request review.BranchRequest
	err     error
	current string
}

func (b *branchStub) ReviewBranch(ctx context.Context, req review.BranchRequest) (review.Result, error) {
	b.request = req
	return review.Result{}, b.err
}

func (b *branchStub) CurrentBranch(ctx context.Context) (string, error) {
	if b.current == "" {
		return "", errors.New("no branch")
	}
	return b.current, nil
}

func TestReviewBranchCommandInvokesUseCase(t *testing.T) {
	stub := &branchStub{}
	root := cli.NewRootCommand(cli.Dependencies{
		BranchReviewer: stub,
		Args:           cli.Arguments{OutWriter: io.Discard, ErrWriter: io.Discard},
		DefaultOutput:  "build",
		DefaultRepo:    "demo",
		Version:        "v1.2.3",
	})

	root.SetArgs([]string{"review", "branch", "feature", "--base", "master", "--include-uncommitted"})
	if err := root.Execute(); err != nil {
		t.Fatalf("command execution failed: %v", err)
	}

	if stub.request.TargetRef != "feature" {
		t.Fatalf("expected target ref feature, got %s", stub.request.TargetRef)
	}

	if stub.request.OutputDir != "build" {
		t.Fatalf("expected default output dir build, got %s", stub.request.OutputDir)
	}

	if !stub.request.IncludeUncommitted {
		t.Fatalf("expected include uncommitted to be true")
	}
}

func TestReviewBranchCommandDetectsTarget(t *testing.T) {
	stub := &branchStub{current: "detected"}
	root := cli.NewRootCommand(cli.Dependencies{
		BranchReviewer: stub,
		Args:           cli.Arguments{OutWriter: io.Discard, ErrWriter: io.Discard},
		DefaultOutput:  "out",
		DefaultRepo:    "demo",
		Version:        "v1.2.3",
	})

	root.SetArgs([]string{"review", "branch", "--base", "master", "--detect-target"})
	if err := root.Execute(); err != nil {
		t.Fatalf("command execution failed: %v", err)
	}

	if stub.request.TargetRef != "detected" {
		t.Fatalf("expected target ref detected, got %s", stub.request.TargetRef)
	}
}

func TestVersionFlagEmitsVersion(t *testing.T) {
	stub := &branchStub{}
	buf := &bytes.Buffer{}
	root := cli.NewRootCommand(cli.Dependencies{
		BranchReviewer: stub,
		Args:           cli.Arguments{OutWriter: buf, ErrWriter: io.Discard},
		Version:        "v9.9.9",
	})

	root.SetArgs([]string{"--version"})
	err := root.Execute()
	if !errors.Is(err, cli.ErrVersionRequested) {
		t.Fatalf("expected version sentinel, got %v", err)
	}
	if strings.TrimSpace(buf.String()) != "v9.9.9" {
		t.Fatalf("unexpected version output: %q", buf.String())
	}
}

func TestVerificationFlags_NoVerifyDisables(t *testing.T) {
	stub := &branchStub{}
	root := cli.NewRootCommand(cli.Dependencies{
		BranchReviewer: stub,
		Args:           cli.Arguments{OutWriter: io.Discard, ErrWriter: io.Discard},
		DefaultVerification: cli.DefaultVerification{
			Enabled:            true, // Config says enabled
			Depth:              "medium",
			CostCeiling:        0.50,
			ConfidenceDefault:  75,
			ConfidenceCritical: 60,
			ConfidenceHigh:     70,
			ConfidenceMedium:   75,
			ConfidenceLow:      85,
		},
		Version: "v1.0.0",
	})

	root.SetArgs([]string{"review", "branch", "main", "--no-verify"})
	if err := root.Execute(); err != nil {
		t.Fatalf("command execution failed: %v", err)
	}

	if !stub.request.SkipVerification {
		t.Error("expected SkipVerification=true when --no-verify is set")
	}
}

func TestVerificationFlags_VerifyEnables(t *testing.T) {
	stub := &branchStub{}
	root := cli.NewRootCommand(cli.Dependencies{
		BranchReviewer: stub,
		Args:           cli.Arguments{OutWriter: io.Discard, ErrWriter: io.Discard},
		DefaultVerification: cli.DefaultVerification{
			Enabled:            false, // Config says disabled
			Depth:              "medium",
			CostCeiling:        0.50,
			ConfidenceDefault:  75,
			ConfidenceCritical: 60,
			ConfidenceHigh:     70,
			ConfidenceMedium:   75,
			ConfidenceLow:      85,
		},
		Version: "v1.0.0",
	})

	root.SetArgs([]string{"review", "branch", "main", "--verify"})
	if err := root.Execute(); err != nil {
		t.Fatalf("command execution failed: %v", err)
	}

	if stub.request.SkipVerification {
		t.Error("expected SkipVerification=false when --verify is set")
	}
}

func TestVerificationFlags_ConfigDefaultUsed(t *testing.T) {
	stub := &branchStub{}
	root := cli.NewRootCommand(cli.Dependencies{
		BranchReviewer: stub,
		Args:           cli.Arguments{OutWriter: io.Discard, ErrWriter: io.Discard},
		DefaultVerification: cli.DefaultVerification{
			Enabled:            true,
			Depth:              "thorough",
			CostCeiling:        1.25,
			ConfidenceDefault:  80,
			ConfidenceCritical: 50,
			ConfidenceHigh:     65,
			ConfidenceMedium:   70,
			ConfidenceLow:      90,
		},
		Version: "v1.0.0",
	})

	root.SetArgs([]string{"review", "branch", "main"})
	if err := root.Execute(); err != nil {
		t.Fatalf("command execution failed: %v", err)
	}

	// Should use config defaults when no CLI flags are set
	if stub.request.SkipVerification {
		t.Error("expected SkipVerification=false (config enabled)")
	}
	if stub.request.VerificationConfig.Depth != "thorough" {
		t.Errorf("expected depth 'thorough', got %q", stub.request.VerificationConfig.Depth)
	}
	if stub.request.VerificationConfig.CostCeiling != 1.25 {
		t.Errorf("expected cost ceiling 1.25, got %f", stub.request.VerificationConfig.CostCeiling)
	}
	if stub.request.VerificationConfig.ConfidenceDefault != 80 {
		t.Errorf("expected confidence default 80, got %d", stub.request.VerificationConfig.ConfidenceDefault)
	}
}

func TestVerificationFlags_CLIOverridesConfig(t *testing.T) {
	stub := &branchStub{}
	root := cli.NewRootCommand(cli.Dependencies{
		BranchReviewer: stub,
		Args:           cli.Arguments{OutWriter: io.Discard, ErrWriter: io.Discard},
		DefaultVerification: cli.DefaultVerification{
			Enabled:            true,
			Depth:              "medium",
			CostCeiling:        0.50,
			ConfidenceDefault:  75,
			ConfidenceCritical: 60,
			ConfidenceHigh:     70,
			ConfidenceMedium:   75,
			ConfidenceLow:      85,
		},
		Version: "v1.0.0",
	})

	root.SetArgs([]string{
		"review", "branch", "main",
		"--verification-depth", "thorough",
		"--verification-cost-ceiling", "2.50",
		"--confidence-default", "85",
		"--confidence-critical", "55",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("command execution failed: %v", err)
	}

	// CLI flags should override config defaults
	if stub.request.VerificationConfig.Depth != "thorough" {
		t.Errorf("expected depth 'thorough', got %q", stub.request.VerificationConfig.Depth)
	}
	if stub.request.VerificationConfig.CostCeiling != 2.50 {
		t.Errorf("expected cost ceiling 2.50, got %f", stub.request.VerificationConfig.CostCeiling)
	}
	if stub.request.VerificationConfig.ConfidenceDefault != 85 {
		t.Errorf("expected confidence default 85, got %d", stub.request.VerificationConfig.ConfidenceDefault)
	}
	if stub.request.VerificationConfig.ConfidenceCritical != 55 {
		t.Errorf("expected confidence critical 55, got %d", stub.request.VerificationConfig.ConfidenceCritical)
	}
	// Non-overridden values should use config defaults
	if stub.request.VerificationConfig.ConfidenceHigh != 70 {
		t.Errorf("expected confidence high 70 (config default), got %d", stub.request.VerificationConfig.ConfidenceHigh)
	}
}

func TestVerificationFlags_InvalidDepthWarns(t *testing.T) {
	stub := &branchStub{}
	errBuf := &bytes.Buffer{}
	root := cli.NewRootCommand(cli.Dependencies{
		BranchReviewer: stub,
		Args:           cli.Arguments{OutWriter: io.Discard, ErrWriter: errBuf},
		DefaultVerification: cli.DefaultVerification{
			Enabled: true,
			Depth:   "medium",
		},
		Version: "v1.0.0",
	})

	root.SetArgs([]string{"review", "branch", "main", "--verification-depth", "invalid"})
	if err := root.Execute(); err != nil {
		t.Fatalf("command execution failed: %v", err)
	}

	// Should warn and fall back to config default
	if !strings.Contains(errBuf.String(), "warning") {
		t.Error("expected warning for invalid depth")
	}
	if stub.request.VerificationConfig.Depth != "medium" {
		t.Errorf("expected fallback to config depth 'medium', got %q", stub.request.VerificationConfig.Depth)
	}
}

func TestVerificationFlags_ZeroCostCeilingAllowed(t *testing.T) {
	stub := &branchStub{}
	root := cli.NewRootCommand(cli.Dependencies{
		BranchReviewer: stub,
		Args:           cli.Arguments{OutWriter: io.Discard, ErrWriter: io.Discard},
		DefaultVerification: cli.DefaultVerification{
			Enabled:     true,
			CostCeiling: 1.50, // Config has non-zero value
		},
		Version: "v1.0.0",
	})

	// Explicitly set cost ceiling to 0 to disable cost limit
	root.SetArgs([]string{"review", "branch", "main", "--verification-cost-ceiling", "0"})
	if err := root.Execute(); err != nil {
		t.Fatalf("command execution failed: %v", err)
	}

	// Should use the explicit zero value, not the config default
	if stub.request.VerificationConfig.CostCeiling != 0 {
		t.Errorf("expected cost ceiling 0, got %f", stub.request.VerificationConfig.CostCeiling)
	}
}

func TestVerificationFlags_ZeroConfidenceAllowed(t *testing.T) {
	stub := &branchStub{}
	root := cli.NewRootCommand(cli.Dependencies{
		BranchReviewer: stub,
		Args:           cli.Arguments{OutWriter: io.Discard, ErrWriter: io.Discard},
		DefaultVerification: cli.DefaultVerification{
			Enabled:           true,
			ConfidenceDefault: 75,
		},
		Version: "v1.0.0",
	})

	// Explicitly set confidence to 0 (verify nothing)
	root.SetArgs([]string{"review", "branch", "main", "--confidence-default", "0"})
	if err := root.Execute(); err != nil {
		t.Fatalf("command execution failed: %v", err)
	}

	// Should use the explicit zero value, not the config default
	if stub.request.VerificationConfig.ConfidenceDefault != 0 {
		t.Errorf("expected confidence default 0, got %d", stub.request.VerificationConfig.ConfidenceDefault)
	}
}

func TestVerificationFlags_NegativeCostCeilingWarns(t *testing.T) {
	stub := &branchStub{}
	errBuf := &bytes.Buffer{}
	root := cli.NewRootCommand(cli.Dependencies{
		BranchReviewer: stub,
		Args:           cli.Arguments{OutWriter: io.Discard, ErrWriter: errBuf},
		DefaultVerification: cli.DefaultVerification{
			Enabled:     true,
			CostCeiling: 1.50,
		},
		Version: "v1.0.0",
	})

	root.SetArgs([]string{"review", "branch", "main", "--verification-cost-ceiling", "-1.0"})
	if err := root.Execute(); err != nil {
		t.Fatalf("command execution failed: %v", err)
	}

	// Should warn and fall back to config default
	if !strings.Contains(errBuf.String(), "warning") {
		t.Errorf("expected warning for negative cost ceiling, got: %q", errBuf.String())
	}
	if stub.request.VerificationConfig.CostCeiling != 1.50 {
		t.Errorf("expected fallback to config cost ceiling 1.50, got %f", stub.request.VerificationConfig.CostCeiling)
	}
}

func TestVerificationFlags_ConfidenceOutOfRangeWarns(t *testing.T) {
	stub := &branchStub{}
	errBuf := &bytes.Buffer{}
	root := cli.NewRootCommand(cli.Dependencies{
		BranchReviewer: stub,
		Args:           cli.Arguments{OutWriter: io.Discard, ErrWriter: errBuf},
		DefaultVerification: cli.DefaultVerification{
			Enabled:            true,
			ConfidenceCritical: 60,
		},
		Version: "v1.0.0",
	})

	// Test value > 100
	root.SetArgs([]string{"review", "branch", "main", "--confidence-critical", "150"})
	if err := root.Execute(); err != nil {
		t.Fatalf("command execution failed: %v", err)
	}

	// Should warn and fall back to safe default (75)
	if !strings.Contains(errBuf.String(), "out of range") {
		t.Errorf("expected 'out of range' warning, got: %q", errBuf.String())
	}
	// Invalid values fall back to safe default (75), not config default
	if stub.request.VerificationConfig.ConfidenceCritical != 75 {
		t.Errorf("expected fallback to safe default 75, got %d", stub.request.VerificationConfig.ConfidenceCritical)
	}
}

func TestVerificationFlags_ConfigDefaultOutOfRangeWarns(t *testing.T) {
	stub := &branchStub{}
	errBuf := &bytes.Buffer{}
	root := cli.NewRootCommand(cli.Dependencies{
		BranchReviewer: stub,
		Args:           cli.Arguments{OutWriter: io.Discard, ErrWriter: errBuf},
		DefaultVerification: cli.DefaultVerification{
			Enabled:            true,
			ConfidenceCritical: 150, // Invalid config default
		},
		Version: "v1.0.0",
	})

	// No CLI flag set - should use config default but validate it
	root.SetArgs([]string{"review", "branch", "main"})
	if err := root.Execute(); err != nil {
		t.Fatalf("command execution failed: %v", err)
	}

	// Should warn about invalid config default
	if !strings.Contains(errBuf.String(), "config") || !strings.Contains(errBuf.String(), "out of range") {
		t.Errorf("expected 'config ... out of range' warning, got: %q", errBuf.String())
	}
	// Should fall back to safe default (75)
	if stub.request.VerificationConfig.ConfidenceCritical != 75 {
		t.Errorf("expected fallback to safe default 75, got %d", stub.request.VerificationConfig.ConfidenceCritical)
	}
}
