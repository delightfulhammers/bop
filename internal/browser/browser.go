// Package browser provides utilities for opening URLs in the user's default browser
// and detecting SSH sessions where browser opening would be inappropriate.
package browser

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// ErrSSHSession is returned when browser opening is skipped due to SSH session detection.
var ErrSSHSession = errors.New("running in SSH session, cannot open browser")

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
func OpenURLWith(url string, isSSH func() bool, run CommandRunner) error {
	if isSSH() {
		return ErrSSHSession
	}

	name, args := browserCommand(url)
	if name == "" {
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	return run(name, args...)
}

// browserCommand returns the platform-specific command and args to open a URL.
func browserCommand(url string) (string, []string) {
	switch runtime.GOOS {
	case "darwin":
		return "open", []string{url}
	case "linux":
		return "xdg-open", []string{url}
	case "windows":
		return "cmd", []string{"/c", "start", url}
	default:
		return "", nil
	}
}

// execRun is the default command runner that uses os/exec.
func execRun(name string, args ...string) error {
	return exec.Command(name, args...).Start()
}

// OpenURL attempts to open the given URL in the default browser using real dependencies.
// In SSH sessions, it returns ErrSSHSession without attempting to launch a browser.
func OpenURL(url string) error {
	return OpenURLWith(
		url,
		func() bool { return IsSSHSessionWith(os.Getenv) },
		execRun,
	)
}
