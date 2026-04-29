// Package clipboard provides terminal clipboard integration via OSC52.
// This works in most modern terminals (iTerm2, Kitty, tmux, foot, alacritty,
// Windows Terminal, VS Code integrated terminal, etc.) without X11/Wayland.
package clipboard

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

const (
	osc52Write = "\x1b]52;c;%s\x07"       // OSC 52 write to system clipboard
	osc52Read  = "\x1b]52;c;?\x07"       // OSC 52 read request
	osc52Reply = "\x1b]52;c;"           // OSC 52 reply prefix
)

// Write copies the given string to the system clipboard via OSC52.
func Write(s string) error {
	enc := base64.StdEncoding.EncodeToString([]byte(s))
	_, err := fmt.Fprintf(os.Stdout, osc52Write, enc)
	return err
}

// Read attempts to read the system clipboard via OSC52.
// This requires the terminal to support OSC52 read (many do not).
// A short timeout is used to avoid hanging indefinitely.
func Read() (string, error) {
	// Send read request
	_, err := os.Stdout.WriteString(osc52Read)
	if err != nil {
		return "", err
	}

	// Try to read the reply from stdin with a timeout
	// This is a best-effort approach; many terminals do not support reading.
	ch := make(chan string, 1)
	go func() {
		var b strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				b.Write(buf[:n])
				data := b.String()
				if idx := strings.Index(data, osc52Reply); idx >= 0 {
					rest := data[idx+len(osc52Reply):]
					if end := strings.IndexByte(rest, '\x07'); end >= 0 {
						enc := rest[:end]
						dec, _ := base64.StdEncoding.DecodeString(enc)
						ch <- string(dec)
						return
					}
				}
			}
			if err != nil {
				if err != io.EOF {
					b.WriteString(err.Error())
				}
				ch <- ""
				return
			}
		}
	}()

	select {
	case result := <-ch:
		if result == "" {
			return "", fmt.Errorf("clipboard read not supported by terminal")
		}
		return result, nil
	case <-time.After(500 * time.Millisecond):
		return "", fmt.Errorf("clipboard read timeout")
	}
}
