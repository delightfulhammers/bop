package cli_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/delightfulhammers/bop/internal/adapter/cli"
)

func TestAuthCommand_HasSubcommands(t *testing.T) {
	root := cli.NewRootCommand(cli.Dependencies{
		BranchReviewer: &branchStub{},
		Args:           cli.Arguments{OutWriter: io.Discard, ErrWriter: io.Discard},
		Version:        "v1.0.0",
	})

	// The auth command should exist and have subcommands
	authCmd, _, err := root.Find([]string{"auth"})
	if err != nil {
		t.Fatalf("expected auth command to exist: %v", err)
	}

	subcommands := authCmd.Commands()
	names := make(map[string]bool)
	for _, sub := range subcommands {
		names[sub.Name()] = true
	}

	for _, expected := range []string{"login", "logout", "whoami", "status"} {
		if !names[expected] {
			t.Errorf("expected auth subcommand %q to exist", expected)
		}
	}
}

func TestAuthStatus_NotLoggedIn(t *testing.T) {
	// This test verifies that `bop auth status` works when not logged in.
	// It relies on there being no credentials file at the default path,
	// which is the case in a test environment (unless the developer running
	// tests is actually logged in — that's acceptable for a non-destructive read).
	out := &bytes.Buffer{}
	root := cli.NewRootCommand(cli.Dependencies{
		BranchReviewer: &branchStub{},
		Args:           cli.Arguments{OutWriter: out, ErrWriter: io.Discard},
		Version:        "v1.0.0",
	})

	root.SetArgs([]string{"auth", "status"})
	err := root.Execute()
	if err != nil {
		t.Fatalf("auth status should not error: %v", err)
	}

	output := out.String()
	// Should show either "not authenticated" or actual status — both are valid
	if output == "" {
		t.Error("expected non-empty output from auth status")
	}
}

func TestAuthLogout_NotLoggedIn(t *testing.T) {
	out := &bytes.Buffer{}
	root := cli.NewRootCommand(cli.Dependencies{
		BranchReviewer: &branchStub{},
		Args:           cli.Arguments{OutWriter: out, ErrWriter: io.Discard},
		Version:        "v1.0.0",
	})

	root.SetArgs([]string{"auth", "logout"})
	err := root.Execute()
	if err != nil {
		t.Fatalf("auth logout should not error when not logged in: %v", err)
	}

	if !strings.Contains(out.String(), "Not logged in") {
		t.Errorf("expected 'Not logged in' message, got: %s", out.String())
	}
}

func TestAuthWhoami_NotLoggedIn(t *testing.T) {
	out := &bytes.Buffer{}
	root := cli.NewRootCommand(cli.Dependencies{
		BranchReviewer: &branchStub{},
		Args:           cli.Arguments{OutWriter: out, ErrWriter: io.Discard},
		Version:        "v1.0.0",
	})

	root.SetArgs([]string{"auth", "whoami"})
	err := root.Execute()
	if err != nil {
		t.Fatalf("auth whoami should not error when not logged in: %v", err)
	}

	if !strings.Contains(out.String(), "Not logged in") {
		t.Errorf("expected 'Not logged in' message, got: %s", out.String())
	}
}

func TestAuthLogin_HasPlatformURLFlag(t *testing.T) {
	root := cli.NewRootCommand(cli.Dependencies{
		BranchReviewer:     &branchStub{},
		Args:               cli.Arguments{OutWriter: io.Discard, ErrWriter: io.Discard},
		DefaultPlatformURL: "https://custom.example.com",
		Version:            "v1.0.0",
	})

	loginCmd, _, err := root.Find([]string{"auth", "login"})
	if err != nil {
		t.Fatalf("expected auth login command to exist: %v", err)
	}

	flag := loginCmd.Flags().Lookup("platform-url")
	if flag == nil {
		t.Fatal("expected --platform-url flag on auth login")
		return
	}
	if flag.DefValue != "https://custom.example.com" {
		t.Errorf("expected default URL 'https://custom.example.com', got %q", flag.DefValue)
	}
}
