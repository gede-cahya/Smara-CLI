package ssh

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/sftp"
)

// TransferMethod defines the protocol used for file transfer.
type TransferMethod string

const (
	TransferMethodAuto  TransferMethod = "auto"
	TransferMethodSFTP  TransferMethod = "sftp"
	TransferMethodSCP   TransferMethod = "scp"
)

// TransferConfig holds options for upload/download operations.
type TransferConfig struct {
	Method        TransferMethod
	Recursive     bool
	PreservePerms bool
}

// TransferResult summarizes a file transfer operation.
type TransferResult struct {
	LocalPath  string
	RemotePath string
	Direction  string // upload / download
	Bytes      int64
	Duration   time.Duration
	Status     string // success / error
	Error      error
}

// expandTilde expands ~ to the user's home directory.
func expandTilde(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

// UploadFile uploads a single file via SFTP.
func UploadFile(client *Client, localPath, remotePath string, preserve bool) (*TransferResult, error) {
	start := time.Now()
	localPath = expandTilde(localPath)

	info, err := os.Stat(localPath)
	if err != nil {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "upload", Status: "error", Error: err, Duration: time.Since(start)}, fmt.Errorf("gagal stat file lokal: %w", err)
	}
	if info.IsDir() {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "upload", Status: "error", Error: fmt.Errorf("path adalah direktori, gunakan UploadDir"), Duration: time.Since(start)}, fmt.Errorf("path adalah direktori, gunakan UploadDir")
	}

	sftpClient, err := sftp.NewClient(client.client)
	if err != nil {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "upload", Status: "error", Error: err, Duration: time.Since(start)}, fmt.Errorf("gagal membuat sftp client: %w", err)
	}
	defer sftpClient.Close()

	// Ensure remote directory exists
	remoteDir := filepath.Dir(remotePath)
	_ = sftpClient.MkdirAll(remoteDir)

	src, err := os.Open(localPath)
	if err != nil {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "upload", Status: "error", Error: err, Duration: time.Since(start)}, fmt.Errorf("gagal membuka file lokal: %w", err)
	}
	defer src.Close()

	dst, err := sftpClient.Create(remotePath)
	if err != nil {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "upload", Status: "error", Error: err, Duration: time.Since(start)}, fmt.Errorf("gagal membuat file remote: %w", err)
	}
	defer dst.Close()

	n, err := io.Copy(dst, src)
	if err != nil {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "upload", Bytes: n, Status: "error", Error: err, Duration: time.Since(start)}, fmt.Errorf("gagal copy file: %w", err)
	}

	if preserve {
		_ = sftpClient.Chmod(remotePath, info.Mode())
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

// DownloadFile downloads a single file via SFTP.
func DownloadFile(client *Client, remotePath, localPath string, preserve bool) (*TransferResult, error) {
	start := time.Now()
	localPath = expandTilde(localPath)

	sftpClient, err := sftp.NewClient(client.client)
	if err != nil {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "download", Status: "error", Error: err, Duration: time.Since(start)}, fmt.Errorf("gagal membuat sftp client: %w", err)
	}
	defer sftpClient.Close()

	remoteInfo, err := sftpClient.Stat(remotePath)
	if err != nil {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "download", Status: "error", Error: err, Duration: time.Since(start)}, fmt.Errorf("gagal stat file remote: %w", err)
	}
	if remoteInfo.IsDir() {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "download", Status: "error", Error: fmt.Errorf("path adalah direktori, gunakan DownloadDir"), Duration: time.Since(start)}, fmt.Errorf("path adalah direktori, gunakan DownloadDir")
	}

	// Ensure local directory exists
	localDir := filepath.Dir(localPath)
	if err := os.MkdirAll(localDir, 0755); err != nil {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "download", Status: "error", Error: err, Duration: time.Since(start)}, fmt.Errorf("gagal membuat direktori lokal: %w", err)
	}

	src, err := sftpClient.Open(remotePath)
	if err != nil {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "download", Status: "error", Error: err, Duration: time.Since(start)}, fmt.Errorf("gagal membuka file remote: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(localPath)
	if err != nil {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "download", Status: "error", Error: err, Duration: time.Since(start)}, fmt.Errorf("gagal membuat file lokal: %w", err)
	}
	defer dst.Close()

	n, err := io.Copy(dst, src)
	if err != nil {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "download", Bytes: n, Status: "error", Error: err, Duration: time.Since(start)}, fmt.Errorf("gagal copy file: %w", err)
	}

	if preserve {
		_ = os.Chmod(localPath, remoteInfo.Mode())
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

// UploadDir recursively uploads a directory via SFTP.
func UploadDir(client *Client, localDir, remoteDir string, preserve bool) ([]TransferResult, error) {
	startAll := time.Now()
	localDir = expandTilde(localDir)

	sftpClient, err := sftp.NewClient(client.client)
	if err != nil {
		return nil, fmt.Errorf("gagal membuat sftp client: %w", err)
	}
	defer sftpClient.Close()

	var results []TransferResult

	err = filepath.Walk(localDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			results = append(results, TransferResult{
				LocalPath: path, Direction: "upload", Status: "error",
				Error: walkErr, Duration: time.Since(startAll),
			})
			return nil // continue walking
		}

		rel, err := filepath.Rel(localDir, path)
		if err != nil {
			return err
		}
		remotePath := filepath.Join(remoteDir, rel)
		// SFTP uses forward slashes on remote
		remotePath = strings.ReplaceAll(remotePath, string(os.PathSeparator), "/")

		if info.IsDir() {
			if err := sftpClient.MkdirAll(remotePath); err != nil {
				results = append(results, TransferResult{
					LocalPath: path, RemotePath: remotePath,
					Direction: "upload", Status: "error", Error: err,
				})
			}
			return nil
		}

		res, err := uploadFileWithSFTP(sftpClient, path, remotePath, info, preserve)
		if err != nil {
			results = append(results, *res)
		} else {
			results = append(results, *res)
		}
		return nil
	})

	return results, err
}

// DownloadDir recursively downloads a directory via SFTP.
func DownloadDir(client *Client, remoteDir, localDir string, preserve bool) ([]TransferResult, error) {
	startAll := time.Now()
	localDir = expandTilde(localDir)

	sftpClient, err := sftp.NewClient(client.client)
	if err != nil {
		return nil, fmt.Errorf("gagal membuat sftp client: %w", err)
	}
	defer sftpClient.Close()

	var results []TransferResult

	walker := sftpClient.Walk(remoteDir)
	for walker.Step() {
		info := walker.Stat()
		remotePath := walker.Path()

		rel, err := filepath.Rel(remoteDir, remotePath)
		if err != nil {
			results = append(results, TransferResult{
				RemotePath: remotePath, Direction: "download", Status: "error",
				Error: err, Duration: time.Since(startAll),
			})
			continue
		}
		localPath := filepath.Join(localDir, rel)

		if info.IsDir() {
			if err := os.MkdirAll(localPath, 0755); err != nil {
				results = append(results, TransferResult{
					RemotePath: remotePath, LocalPath: localPath,
					Direction: "download", Status: "error", Error: err,
				})
			}
			continue
		}

		res, _ := downloadFileWithSFTP(sftpClient, remotePath, localPath, info, preserve)
		results = append(results, *res)
	}

	if err := walker.Err(); err != nil {
		return results, err
	}
	return results, nil
}

func uploadFileWithSFTP(sftpClient *sftp.Client, localPath, remotePath string, info os.FileInfo, preserve bool) (*TransferResult, error) {
	start := time.Now()

	src, err := os.Open(localPath)
	if err != nil {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "upload", Status: "error", Error: err, Duration: time.Since(start)}, err
	}
	defer src.Close()

	// Ensure parent dir exists
	remoteParent := filepath.Dir(remotePath)
	remoteParent = strings.ReplaceAll(remoteParent, string(os.PathSeparator), "/")
	_ = sftpClient.MkdirAll(remoteParent)

	dst, err := sftpClient.Create(remotePath)
	if err != nil {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "upload", Status: "error", Error: err, Duration: time.Since(start)}, err
	}
	defer dst.Close()

	n, err := io.Copy(dst, src)
	if err != nil {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "upload", Bytes: n, Status: "error", Error: err, Duration: time.Since(start)}, err
	}

	if preserve {
		_ = sftpClient.Chmod(remotePath, info.Mode())
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

func downloadFileWithSFTP(sftpClient *sftp.Client, remotePath, localPath string, info os.FileInfo, preserve bool) (*TransferResult, error) {
	start := time.Now()

	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "download", Status: "error", Error: err, Duration: time.Since(start)}, err
	}

	src, err := sftpClient.Open(remotePath)
	if err != nil {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "download", Status: "error", Error: err, Duration: time.Since(start)}, err
	}
	defer src.Close()

	dst, err := os.Create(localPath)
	if err != nil {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "download", Status: "error", Error: err, Duration: time.Since(start)}, err
	}
	defer dst.Close()

	n, err := io.Copy(dst, src)
	if err != nil {
		return &TransferResult{LocalPath: localPath, RemotePath: remotePath, Direction: "download", Bytes: n, Status: "error", Error: err, Duration: time.Since(start)}, err
	}

	if preserve {
		_ = os.Chmod(localPath, info.Mode())
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
