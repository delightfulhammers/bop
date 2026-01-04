//go:build mage

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

var (
	// Default target executed when none is specified.
	Default = CI
)

// =============================================================================
// Composite Targets
// =============================================================================

// CI runs the full CI pipeline: format, lint, test (with race), build all.
func CI() {
	mg.SerialDeps(Format, Lint, TestRace, BuildAll)
}

// Check runs quick pre-commit checks: format, vet, test (no race).
func Check() {
	mg.SerialDeps(Format, Vet, Test)
}

// All runs everything: format, lint, test with race and coverage, build all.
func All() {
	mg.SerialDeps(Format, Lint, TestRace, TestCoverage, BuildAll)
}

// =============================================================================
// Build Targets
// =============================================================================

// Build compiles the main bop binary with version info.
func Build() error {
	fmt.Println("==> Building bop binary...")
	return buildBinary("bop", "./cmd/bop")
}

// BuildMCP compiles the bop-mcp binary with version info.
func BuildMCP() error {
	fmt.Println("==> Building bop-mcp binary...")
	return buildBinary("bop-mcp", "./cmd/bop-mcp")
}

// BuildAll compiles all binaries (bop and bop-mcp).
func BuildAll() error {
	fmt.Println("==> Building all binaries...")
	// First verify all packages compile
	if err := run("go", "build", "./..."); err != nil {
		return err
	}
	if err := Build(); err != nil {
		return err
	}
	return BuildMCP()
}

// Install installs all binaries to $GOPATH/bin.
func Install() error {
	fmt.Println("==> Installing binaries to GOPATH/bin...")
	version := resolveVersion()
	ldflags := fmt.Sprintf("-X github.com/delightfulhammers/bop/internal/version.version=%s", version)

	if err := run("go", "install", "-ldflags", ldflags, "./cmd/bop"); err != nil {
		return err
	}
	return run("go", "install", "-ldflags", ldflags, "./cmd/bop-mcp")
}

// Clean removes build artifacts.
func Clean() error {
	fmt.Println("==> Cleaning build artifacts...")
	// Remove artifacts from root directory
	artifacts := []string{"bop", "bop-mcp", "coverage.out", "coverage.html"}
	for _, artifact := range artifacts {
		if err := os.Remove(artifact); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove %s: %w", artifact, err)
		}
	}
	// Also remove artifacts from bin/ directory (buildBinary outputs there if it exists)
	binArtifacts := []string{"bin/bop", "bin/bop-mcp"}
	for _, artifact := range binArtifacts {
		if err := os.Remove(artifact); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove %s: %w", artifact, err)
		}
	}
	return nil
}

// =============================================================================
// Test Targets
// =============================================================================

// Test runs the full Go test suite (no race detector, faster).
func Test() error {
	fmt.Println("==> Running tests...")
	return run("go", "test", "./...")
}

// TestRace runs the full Go test suite with race detection enabled.
func TestRace() error {
	fmt.Println("==> Running tests with race detector...")
	return run("go", "test", "-race", "./...")
}

// TestCoverage runs tests with coverage report and generates HTML output.
func TestCoverage() error {
	fmt.Println("==> Running tests with coverage...")
	if err := run("go", "test", "-coverprofile=coverage.out", "-covermode=atomic", "./..."); err != nil {
		return err
	}
	fmt.Println("==> Generating coverage HTML report...")
	if err := run("go", "tool", "cover", "-html=coverage.out", "-o", "coverage.html"); err != nil {
		return err
	}
	fmt.Println("Coverage report: coverage.html")
	return nil
}

// TestUnit runs only unit tests (excludes integration/e2e tests).
// Uses build tags to filter tests.
func TestUnit() error {
	fmt.Println("==> Running unit tests...")
	return run("go", "test", "-short", "./...")
}

// TestVerbose runs tests with verbose output.
func TestVerbose() error {
	fmt.Println("==> Running tests (verbose)...")
	return run("go", "test", "-v", "./...")
}

// =============================================================================
// Code Quality Targets
// =============================================================================

// Format updates Go sources using gofmt.
func Format() error {
	fmt.Println("==> Formatting code...")
	return run("gofmt", "-w", ".")
}

// Vet runs go vet for static analysis.
func Vet() error {
	fmt.Println("==> Running go vet...")
	return run("go", "vet", "./...")
}

// Lint runs golangci-lint for comprehensive static analysis.
// Requires golangci-lint to be installed.
func Lint() error {
	fmt.Println("==> Running golangci-lint...")
	if _, err := exec.LookPath("golangci-lint"); err != nil {
		fmt.Println("Warning: golangci-lint not found, falling back to go vet")
		return Vet()
	}
	return run("golangci-lint", "run")
}

// LintFix runs golangci-lint with auto-fix enabled.
func LintFix() error {
	fmt.Println("==> Running golangci-lint with auto-fix...")
	if _, err := exec.LookPath("golangci-lint"); err != nil {
		return fmt.Errorf("golangci-lint not found; install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest")
	}
	return run("golangci-lint", "run", "--fix")
}

// =============================================================================
// Development Helpers
// =============================================================================

// Deps downloads and verifies module dependencies.
func Deps() error {
	fmt.Println("==> Downloading dependencies...")
	if err := run("go", "mod", "download"); err != nil {
		return err
	}
	fmt.Println("==> Verifying dependencies...")
	return run("go", "mod", "verify")
}

// Tidy cleans up go.mod and go.sum.
func Tidy() error {
	fmt.Println("==> Tidying modules...")
	return run("go", "mod", "tidy")
}

// Generate runs go generate for all packages.
func Generate() error {
	fmt.Println("==> Running go generate...")
	return run("go", "generate", "./...")
}

// =============================================================================
// Helper Functions
// =============================================================================

func buildBinary(name, pkg string) error {
	version := resolveVersion()
	ldflags := fmt.Sprintf("-X github.com/delightfulhammers/bop/internal/version.version=%s", version)

	outputPath := name
	if _, err := os.Stat("bin"); err == nil {
		outputPath = filepath.Join("bin", name)
	}

	return run("go", "build", "-ldflags", ldflags, "-o", outputPath, pkg)
}

func run(cmd string, args ...string) error {
	if err := sh.RunV(cmd, args...); err != nil {
		return fmt.Errorf("%s %v: %w", cmd, args, err)
	}
	return nil
}

func resolveVersion() string {
	const defaultVersion = "v0.0.0"

	tag, err := gitOutput("describe", "--tags", "--abbrev=0")
	if err != nil {
		return defaultVersion
	}
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return defaultVersion
	}

	if repoDirty() {
		return tag + "-dirty"
	}

	if !headMatchesTag() {
		return tag + "-dirty"
	}

	return tag
}

func repoDirty() bool {
	output, err := gitOutput("status", "--porcelain")
	if err != nil {
		return false
	}
	return strings.TrimSpace(output) != ""
}

func headMatchesTag() bool {
	_, err := gitOutput("describe", "--tags", "--exact-match")
	if err != nil {
		errText := err.Error()
		switch {
		case strings.Contains(errText, "no tag exactly matches"),
			strings.Contains(errText, "no names found"):
			return false
		default:
			return false
		}
	}
	return true
}

func gitOutput(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		if stderr.Len() > 0 {
			err = fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return "", err
	}
	return stdout.String(), nil
}
