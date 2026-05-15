// Package clipboard provides terminal clipboard integration.
//
// Write uses OSC 52 escape sequences (works in iTerm2, Kitty, tmux, foot,
// alacritty, Windows Terminal, VS Code integrated terminal, even over SSH)
// with a fallback to native clipboard utilities (xclip / wl-copy / pbcopy /
// clip.exe) via the github.com/atotto/clipboard package.
//
// Read uses ONLY the native clipboard utilities. The previous OSC 52 read
// implementation spawned a goroutine that consumed bytes from os.Stdin,
// which conflicts with Bubble Tea's raw-mode input loop and froze the TUI.
// We never read stdin from this package now.
package clipboard

import (
	"encoding/base64"
	"fmt"
	"os"

	atotto "github.com/atotto/clipboard"
)

// osc52WriteFmt writes the base64-encoded payload to the system clipboard.
const osc52WriteFmt = "\x1b]52;c;%s\x07"

// Write copies the given string to the system clipboard.
//
// Strategy:
//  1. Emit OSC 52 to stdout (best-effort; works over SSH and inside tmux).
//  2. Also try the native clipboard utility so apps that don't honor OSC 52
//     still get the value. Either succeeding is treated as success.
func Write(s string) error {
	enc := base64.StdEncoding.EncodeToString([]byte(s))
	_, oscErr := fmt.Fprintf(os.Stdout, osc52WriteFmt, enc)

	nativeErr := atotto.WriteAll(s)

	if oscErr == nil || nativeErr == nil {
		return nil
	}
	return fmt.Errorf("clipboard write gagal: osc52=%v, native=%v", oscErr, nativeErr)
}

// Read returns the current text contents of the system clipboard.
//
// Uses xclip / wl-paste / pbpaste / clip.exe under the hood. Returns an
// error if no clipboard helper is available on the host (e.g. headless
// Linux without xclip or wl-clipboard installed).
//
// Important: this function never reads from os.Stdin, so it is safe to
// call from inside a Bubble Tea program where stdin is already in raw mode.
func Read() (string, error) {
	if !atotto.Unsupported {
		s, err := atotto.ReadAll()
		if err != nil {
			return "", fmt.Errorf("clipboard read gagal: %w", err)
		}
		return s, nil
	}
	return "", fmt.Errorf("clipboard tidak didukung di platform ini (install xclip / wl-clipboard)")
}
