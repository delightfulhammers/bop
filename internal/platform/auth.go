package platform

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"time"
)

// Login orchestrates the device flow: initiates, prompts the user, polls for
// the token, and returns credentials on success.
func Login(ctx context.Context, client *Client, productID string, out io.Writer) (*Credentials, error) {
	flowResp, err := client.InitiateDeviceFlow(ctx, productID, "github")
	if err != nil {
		return nil, fmt.Errorf("start device flow: %w", err)
	}

	_, _ = fmt.Fprintf(out, "\nTo authenticate, open the following URL in your browser:\n\n")
	_, _ = fmt.Fprintf(out, "  %s\n\n", flowResp.VerificationURI)
	_, _ = fmt.Fprintf(out, "Then enter this code: %s\n\n", flowResp.UserCode)

	// Best-effort browser open
	openBrowser(flowResp.VerificationURI)

	_, _ = fmt.Fprintf(out, "Waiting for authorization...")

	interval := time.Duration(flowResp.Interval) * time.Second
	if interval < time.Second {
		interval = 5 * time.Second
	}
	deadline := time.Now().Add(time.Duration(flowResp.ExpiresIn) * time.Second)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}

		if time.Now().After(deadline) {
			_, _ = fmt.Fprintln(out, " timed out.")
			return nil, fmt.Errorf("device flow expired — please try again")
		}

		tokenResp, err := client.PollDeviceToken(ctx, productID, flowResp.DeviceCode)
		if err != nil {
			_, _ = fmt.Fprintln(out, " failed.")
			return nil, err
		}
		if tokenResp == nil {
			// authorization_pending — keep polling
			continue
		}

		_, _ = fmt.Fprintln(out, " done!")

		creds := &Credentials{
			AccessToken:  tokenResp.AccessToken,
			RefreshToken: tokenResp.RefreshToken,
			ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
			UserID:       tokenResp.UserID,
			Username:     tokenResp.Username,
			PlatformURL:  client.baseURL,
		}
		return creds, nil
	}
}

// openBrowser attempts to open a URL in the user's default browser. Best-effort, never errors.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return
	}
	_ = cmd.Start()
}
