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
	noSSH := func() bool { return false }
	inSSH := func() bool { return true }
	noopRunner := func(name string, args ...string) error { return nil }

	t.Run("returns ErrSSHSession when in SSH session", func(t *testing.T) {
		err := OpenURLWith("https://github.com/login/device", "darwin", inSSH, noopRunner)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrSSHSession))
	})

	t.Run("does not call runner when in SSH session", func(t *testing.T) {
		called := false
		run := func(name string, args ...string) error {
			called = true
			return nil
		}

		_ = OpenURLWith("https://github.com/login/device", "darwin", inSSH, run)
		assert.False(t, called, "command runner should not be called in SSH session")
	})

	t.Run("calls command runner with correct args on darwin", func(t *testing.T) {
		var calledName string
		var calledArgs []string
		run := func(name string, args ...string) error {
			calledName = name
			calledArgs = args
			return nil
		}

		err := OpenURLWith("https://github.com/login/device", "darwin", noSSH, run)
		require.NoError(t, err)
		assert.Equal(t, "open", calledName)
		assert.Equal(t, []string{"https://github.com/login/device"}, calledArgs)
	})

	t.Run("calls command runner with correct args on linux", func(t *testing.T) {
		var calledName string
		var calledArgs []string
		run := func(name string, args ...string) error {
			calledName = name
			calledArgs = args
			return nil
		}

		err := OpenURLWith("https://github.com/login/device", "linux", noSSH, run)
		require.NoError(t, err)
		assert.Equal(t, "xdg-open", calledName)
		assert.Equal(t, []string{"https://github.com/login/device"}, calledArgs)
	})

	t.Run("calls rundll32 on windows to avoid cmd shell injection", func(t *testing.T) {
		var calledName string
		var calledArgs []string
		run := func(name string, args ...string) error {
			calledName = name
			calledArgs = args
			return nil
		}

		err := OpenURLWith("https://github.com/login/device?code=ABC", "windows", noSSH, run)
		require.NoError(t, err)
		assert.Equal(t, "rundll32", calledName)
		assert.Equal(t, []string{"url.dll,FileProtocolHandler", "https://github.com/login/device?code=ABC"}, calledArgs)
	})

	t.Run("returns error for unsupported platform", func(t *testing.T) {
		err := OpenURLWith("https://example.com", "plan9", noSSH, noopRunner)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrUnsupportedOS))
		assert.Contains(t, err.Error(), "plan9")
	})

	t.Run("propagates command runner errors", func(t *testing.T) {
		expectedErr := errors.New("command not found")
		run := func(name string, args ...string) error { return expectedErr }

		err := OpenURLWith("https://github.com/login/device", "darwin", noSSH, run)
		require.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})

	t.Run("rejects non-http/https URLs", func(t *testing.T) {
		err := OpenURLWith("file:///etc/passwd", "darwin", noSSH, noopRunner)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidURL))
	})

	t.Run("rejects javascript scheme", func(t *testing.T) {
		err := OpenURLWith("javascript:alert(1)", "darwin", noSSH, noopRunner)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidURL))
	})

	t.Run("rejects empty scheme", func(t *testing.T) {
		err := OpenURLWith("not-a-url", "darwin", noSSH, noopRunner)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidURL))
	})

	t.Run("accepts http URL", func(t *testing.T) {
		err := OpenURLWith("http://localhost:8080/callback", "darwin", noSSH, noopRunner)
		require.NoError(t, err)
	})

	t.Run("accepts https URL", func(t *testing.T) {
		err := OpenURLWith("https://github.com/login/device", "darwin", noSSH, noopRunner)
		require.NoError(t, err)
	})
}

func TestBrowserCommandFor(t *testing.T) {
	t.Run("darwin uses open", func(t *testing.T) {
		name, args := browserCommandFor("darwin", "https://example.com")
		assert.Equal(t, "open", name)
		assert.Equal(t, []string{"https://example.com"}, args)
	})

	t.Run("linux uses xdg-open", func(t *testing.T) {
		name, args := browserCommandFor("linux", "https://example.com")
		assert.Equal(t, "xdg-open", name)
		assert.Equal(t, []string{"https://example.com"}, args)
	})

	t.Run("windows uses rundll32 to avoid shell injection", func(t *testing.T) {
		name, args := browserCommandFor("windows", "https://example.com?a=1&b=2")
		assert.Equal(t, "rundll32", name)
		assert.Equal(t, []string{"url.dll,FileProtocolHandler", "https://example.com?a=1&b=2"}, args)
	})

	t.Run("unsupported OS returns empty", func(t *testing.T) {
		name, args := browserCommandFor("plan9", "https://example.com")
		assert.Empty(t, name)
		assert.Nil(t, args)
	})
}

func TestValidateURL(t *testing.T) {
	t.Run("accepts https", func(t *testing.T) {
		assert.NoError(t, validateURL("https://github.com/login/device"))
	})

	t.Run("accepts http", func(t *testing.T) {
		assert.NoError(t, validateURL("http://localhost:8080"))
	})

	t.Run("rejects file scheme", func(t *testing.T) {
		err := validateURL("file:///etc/passwd")
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidURL))
	})

	t.Run("rejects empty scheme", func(t *testing.T) {
		err := validateURL("no-scheme")
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidURL))
	})
}
