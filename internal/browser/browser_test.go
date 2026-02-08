package browser

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsSSHSessionWith(t *testing.T) {
	t.Run("returns false when no SSH env vars are set", func(t *testing.T) {
		lookup := func(key string) string { return "" }
		assert.False(t, IsSSHSessionWith(lookup))
	})

	t.Run("returns true when SSH_CLIENT is set", func(t *testing.T) {
		lookup := func(key string) string {
			if key == "SSH_CLIENT" {
				return "192.168.1.1 12345 22"
			}
			return ""
		}
		assert.True(t, IsSSHSessionWith(lookup))
	})

	t.Run("returns true when SSH_TTY is set", func(t *testing.T) {
		lookup := func(key string) string {
			if key == "SSH_TTY" {
				return "/dev/pts/0"
			}
			return ""
		}
		assert.True(t, IsSSHSessionWith(lookup))
	})

	t.Run("returns true when SSH_CONNECTION is set", func(t *testing.T) {
		lookup := func(key string) string {
			if key == "SSH_CONNECTION" {
				return "192.168.1.1 12345 192.168.1.2 22"
			}
			return ""
		}
		assert.True(t, IsSSHSessionWith(lookup))
	})

	t.Run("returns true when multiple SSH env vars are set", func(t *testing.T) {
		lookup := func(key string) string {
			switch key {
			case "SSH_CLIENT":
				return "192.168.1.1 12345 22"
			case "SSH_TTY":
				return "/dev/pts/0"
			case "SSH_CONNECTION":
				return "192.168.1.1 12345 192.168.1.2 22"
			default:
				return ""
			}
		}
		assert.True(t, IsSSHSessionWith(lookup))
	})
}

func TestOpenURLWith(t *testing.T) {
	t.Run("returns ErrSSHSession when in SSH session", func(t *testing.T) {
		isSSH := func() bool { return true }
		run := func(name string, args ...string) error { return nil }

		err := OpenURLWith("https://github.com/login/device", isSSH, run)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrSSHSession))
	})

	t.Run("calls command runner when not in SSH session", func(t *testing.T) {
		isSSH := func() bool { return false }
		var calledName string
		var calledArgs []string
		run := func(name string, args ...string) error {
			calledName = name
			calledArgs = args
			return nil
		}

		err := OpenURLWith("https://github.com/login/device", isSSH, run)
		require.NoError(t, err)
		assert.NotEmpty(t, calledName, "expected a command to be called")
		assert.Contains(t, calledArgs, "https://github.com/login/device")
	})

	t.Run("propagates command runner errors", func(t *testing.T) {
		isSSH := func() bool { return false }
		expectedErr := errors.New("command not found")
		run := func(name string, args ...string) error { return expectedErr }

		err := OpenURLWith("https://github.com/login/device", isSSH, run)
		require.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})

	t.Run("does not call runner when in SSH session", func(t *testing.T) {
		isSSH := func() bool { return true }
		called := false
		run := func(name string, args ...string) error {
			called = true
			return nil
		}

		_ = OpenURLWith("https://github.com/login/device", isSSH, run)
		assert.False(t, called, "command runner should not be called in SSH session")
	})
}

func TestBrowserCommand(t *testing.T) {
	// browserCommand is platform-dependent, so we just verify it returns something
	// on the current platform (which must be darwin, linux, or windows for CI)
	name, args := browserCommand("https://example.com")
	assert.NotEmpty(t, name, "expected a browser command on supported platform")
	assert.NotEmpty(t, args, "expected browser command args")
}
