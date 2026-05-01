package ssh

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// SCP single-file upload using ssh channel (scp -t).
func UploadFileSCP(client *Client, localPath, remotePath string, preserve bool) (*TransferResult, error) {
	start := time.Now()
	localPath = expandTilde(localPath)

	info, err := os.Stat(localPath)
	if err != nil {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "upload", Status: "error", Error: err, Duration: time.Since(start)}, fmt.Errorf("gagal stat file lokal: %w", err)
	}
	if info.IsDir() {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "upload", Status: "error", Error: fmt.Errorf("SCP tidak mendukung direktori rekursif"), Duration: time.Since(start)}, fmt.Errorf("SCP tidak mendukung direktori rekursif")
	}

	session, err := client.client.NewSession()
	if err != nil {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "upload", Status: "error", Error: err, Duration: time.Since(start)}, fmt.Errorf("gagal membuat session: %w", err)
	}
	defer session.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "upload", Status: "error", Error: err, Duration: time.Since(start)}, fmt.Errorf("gagal ambil stdin pipe: %w", err)
	}
	defer stdin.Close()

	stdout, err := session.StdoutPipe()
	if err != nil {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "upload", Status: "error", Error: err, Duration: time.Since(start)}, fmt.Errorf("gagal ambil stdout pipe: %w", err)
	}

	// scp -t writes confirmation bytes to stdout; read first one
	go func() {
		_ = session.Run("scp -t " + shellescape(remotePath))
	}()

	// Wait for initial 0 byte
	if err := scpConfirm(stdout); err != nil {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "upload", Status: "error", Error: err, Duration: time.Since(start)}, err
	}

	// Send file directive: C{mode} {size} {filename}
	mode := "0644"
	if preserve {
		mode = fmt.Sprintf("%04o", info.Mode().Perm())
	}
	size := info.Size()
	filename := filepath.Base(remotePath)
	_, _ = fmt.Fprintf(stdin, "C%s %d %s\n", mode, size, filename)

	if err := scpConfirm(stdout); err != nil {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "upload", Status: "error", Error: err, Duration: time.Since(start)}, err
	}

	file, err := os.Open(localPath)
	if err != nil {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "upload", Status: "error", Error: err, Duration: time.Since(start)}, fmt.Errorf("gagal buka file lokal: %w", err)
	}
	n, err := io.Copy(stdin, file)
	file.Close()
	if err != nil {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "upload", Bytes: n, Status: "error", Error: err, Duration: time.Since(start)}, fmt.Errorf("gagal kirim file: %w", err)
	}

	// Send null byte to signal end of file
	_, _ = stdin.Write([]byte{0})

	if err := scpConfirm(stdout); err != nil {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "upload", Bytes: n, Status: "error", Error: err, Duration: time.Since(start)}, err
	}

	return &TransferResult{
		LocalPath:  localPath,
		RemotePath: remotePath,
		Direction:  "upload",
		Bytes:      n,
		Status:     "success",
		Duration:   time.Since(start),
	}, nil
}

// SCP single-file download using ssh channel (scp -f).
func DownloadFileSCP(client *Client, remotePath, localPath string, preserve bool) (*TransferResult, error) {
	start := time.Now()
	localPath = expandTilde(localPath)

	session, err := client.client.NewSession()
	if err != nil {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "download", Status: "error", Error: err, Duration: time.Since(start)}, fmt.Errorf("gagal membuat session: %w", err)
	}
	defer session.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "download", Status: "error", Error: err, Duration: time.Since(start)}, fmt.Errorf("gagal ambil stdin pipe: %w", err)
	}
	defer stdin.Close()

	stdout, err := session.StdoutPipe()
	if err != nil {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "download", Status: "error", Error: err, Duration: time.Since(start)}, fmt.Errorf("gagal ambil stdout pipe: %w", err)
	}

	go func() {
		_ = session.Run("scp -f " + shellescape(remotePath))
	}()

	// Send initial confirmation
	_, _ = stdin.Write([]byte{0})

	// Read control line: C{mode} {size} {filename}
	var ctrlLine string
	buf := make([]byte, 1)
	for {
		_, err := stdout.Read(buf)
		if err != nil {
			return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "download", Status: "error", Error: err, Duration: time.Since(start)}, fmt.Errorf("gagal baca kontrol scp: %w", err)
		}
		if buf[0] == '\n' {
			break
		}
		if buf[0] == 0x01 || buf[0] == 0x02 {
			// Error/warning byte from scp
			return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "download", Status: "error", Error: fmt.Errorf("scp error byte: %d", buf[0]), Duration: time.Since(start)}, fmt.Errorf("scp error byte: %d", buf[0])
		}
		ctrlLine += string(buf)
	}

	if len(ctrlLine) < 1 || ctrlLine[0] != 'C' {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "download", Status: "error", Error: fmt.Errorf("format scp tidak dikenali: %s", ctrlLine), Duration: time.Since(start)}, fmt.Errorf("format scp tidak dikenali: %s", ctrlLine)
	}

	parts := strings.Fields(ctrlLine[1:])
	if len(parts) < 2 {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "download", Status: "error", Error: fmt.Errorf("format scp tidak valid: %s", ctrlLine), Duration: time.Since(start)}, fmt.Errorf("format scp tidak valid: %s", ctrlLine)
	}

	size, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "download", Status: "error", Error: err, Duration: time.Since(start)}, fmt.Errorf("gagal parse ukuran file: %w", err)
	}

	// Confirm ready to receive
	_, _ = stdin.Write([]byte{0})

	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "download", Status: "error", Error: err, Duration: time.Since(start)}, fmt.Errorf("gagal membuat direktori lokal: %w", err)
	}

	file, err := os.Create(localPath)
	if err != nil {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "download", Status: "error", Error: err, Duration: time.Since(start)}, fmt.Errorf("gagal membuat file lokal: %w", err)
	}
	defer file.Close()

	n, err := io.CopyN(file, stdout, size)
	if err != nil {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "download", Bytes: n, Status: "error", Error: err, Duration: time.Since(start)}, fmt.Errorf("gagal menerima file: %w", err)
	}

	// Send confirmation after receiving file
	_, _ = stdin.Write([]byte{0})

	if preserve && len(parts) >= 1 {
		mode, _ := strconv.ParseUint(parts[0], 8, 32)
		_ = os.Chmod(localPath, os.FileMode(mode))
	}

	return &TransferResult{
		LocalPath:  localPath,
		RemotePath: remotePath,
		Direction:  "download",
		Bytes:      n,
		Status:     "success",
		Duration:   time.Since(start),
	}, nil
}

// scpConfirm reads a single 0 byte from scp stdout.
func scpConfirm(r io.Reader) error {
	buf := make([]byte, 1)
	_, err := r.Read(buf)
	if err != nil {
		return fmt.Errorf("scp confirm read error: %w", err)
	}
	if buf[0] != 0 {
		// Could be error message
		return fmt.Errorf("scp error: %d", buf[0])
	}
	return nil
}

// shellescape does minimal escaping for remote scp paths.
func shellescape(s string) string {
	if strings.ContainsAny(s, "'`!$\\;&|()<>") {
		return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
	}
	return s
}
