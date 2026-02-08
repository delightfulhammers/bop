// Package browser provides utilities for opening URLs in the user's default browser
// and detecting SSH sessions where browser opening would be inappropriate.
package browser

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"runtime"
)

// Sentinel errors for browser opening failures.
var (
	ErrSSHSession    = errors.New("running in SSH session, cannot open browser")
	ErrUnsupportedOS = errors.New("unsupported platform")
	ErrInvalidURL    = errors.New("invalid URL: must be http or https")
)

// sshEnvVars are the environment variables that indicate an SSH session.
// These are set by OpenSSH when a remote session is established.
var sshEnvVars = []string{"SSH_CLIENT", "SSH_TTY", "SSH_CONNECTION"}

// EnvLookup is a function that retrieves an environment variable value.
type EnvLookup func(string) string

// CommandRunner executes a command with arguments, returning any error.
type CommandRunner func(name string, args ...string) error

// IsSSHSessionWith checks whether the current process is running in an SSH session
// using the provided environment variable lookup function.
func IsSSHSessionWith(lookup EnvLookup) bool {
	for _, key := range sshEnvVars {
		if lookup(key) != "" {
			return true
		}
	}
	return false
}

// OpenURLWith attempts to open the given URL in the default browser.
// It uses the provided SSH detection and command runner functions for testability.
// Returns nil if the browser command was launched, or an error explaining why not.
func OpenURLWith(rawURL string, goos string, isSSH func() bool, run CommandRunner) error {
	if isSSH() {
		return ErrSSHSession
	}

	if err := validateURL(rawURL); err != nil {
		return err
	}

	name, args := browserCommandFor(goos, rawURL)
	if name == "" {
		return fmt.Errorf("%w: %s", ErrUnsupportedOS, goos)
	}

	return run(name, args...)
}

// validateURL checks that the URL is a valid http or https URL.
func validateURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidURL, rawURL)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%w: scheme %q", ErrInvalidURL, parsed.Scheme)
	}
	return nil
}

// browserCommandFor returns the platform-specific command and args to open a URL.
func browserCommandFor(goos string, urlStr string) (string, []string) {
	switch goos {
	case "darwin":
		return "open", []string{urlStr}
	case "linux":
		return "xdg-open", []string{urlStr}
	case "windows":
		// Empty string as second arg is the window title for "start".
		// Without it, "start" treats the first quoted argument as a title.
		return "rundll32", []string{"url.dll,FileProtocolHandler", urlStr}
	default:
		return "", nil
	}
}

// execRun is the default command runner that uses os/exec.
// It starts the browser process and reaps it in a background goroutine
// to avoid zombie processes.
func execRun(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait() //nolint:errcheck // best-effort reap; browser exit status is not actionable
	return nil
}

// currentOS returns the runtime GOOS value.
func currentOS() string { return runtime.GOOS }

// OpenURL attempts to open the given URL in the default browser using real dependencies.
// In SSH sessions, it returns ErrSSHSession without attempting to launch a browser.
func OpenURL(rawURL string) error {
	return OpenURLWith(
		rawURL,
		currentOS(),
		func() bool { return IsSSHSessionWith(os.Getenv) },
		execRun,
	)
}
