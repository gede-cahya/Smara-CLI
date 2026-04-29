package ssh

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

// Client wraps an SSH client connection.
type Client struct {
	client  *gossh.Client
	session *gossh.Session
}

// Connect establishes an SSH connection to the given host.
func Connect(host *Host) (*Client, error) {
	if err := validateHost(host); err != nil {
		return nil, err
	}

	// Check port 22 is open (lightweight validation)
	if host.Port == "" {
		host.Port = "22"
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host.Address, host.Port), 3*time.Second)
	if err != nil {
		return nil, fmt.Errorf("host tidak merespon pada port %s: %w", host.Port, err)
	}
	conn.Close()

	config, err := buildSSHConfig(host)
	if err != nil {
		return nil, fmt.Errorf("gagal membangun konfigurasi SSH: %w", err)
	}

	client, err := gossh.Dial("tcp", net.JoinHostPort(host.Address, host.Port), config)
	if err != nil {
		return nil, fmt.Errorf("gagal koneksi SSH: %w", err)
	}

	return &Client{client: client}, nil
}

// Exec runs a command on the remote host and returns stdout and stderr.
func (c *Client) Exec(command string) (stdout, stderr string, err error) {
	session, err := c.client.NewSession()
	if err != nil {
		return "", "", fmt.Errorf("gagal membuat session: %w", err)
	}
	defer session.Close()

	var outBuf, errBuf bytes.Buffer
	session.Stdout = &outBuf
	session.Stderr = &errBuf

	if err := session.Run(command); err != nil {
		return outBuf.String(), errBuf.String(), fmt.Errorf("eksekusi gagal: %w", err)
	}

	return outBuf.String(), errBuf.String(), nil
}

// ExecStream runs a command and streams output through a callback.
func (c *Client) ExecStream(command string, onOutput func(line string, isStderr bool)) error {
	session, err := c.client.NewSession()
	if err != nil {
		return fmt.Errorf("gagal membuat session: %w", err)
	}
	defer session.Close()

	stdoutPipe, err := session.StdoutPipe()
	if err != nil {
		return fmt.Errorf("gagal mengambil stdout: %w", err)
	}
	stderrPipe, err := session.StderrPipe()
	if err != nil {
		return fmt.Errorf("gagal mengambil stderr: %w", err)
	}

	if err := session.Start(command); err != nil {
		return fmt.Errorf("gagal memulai command: %w", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	readPipe := func(pipe io.Reader, isStderr bool) {
		defer wg.Done()
		scanner := bufio.NewScanner(pipe)
		for scanner.Scan() {
			if onOutput != nil {
				onOutput(scanner.Text(), isStderr)
			}
		}
	}

	go readPipe(stdoutPipe, false)
	go readPipe(stderrPipe, true)

	wg.Wait()
	return session.Wait()
}

// InteractiveSession starts an interactive PTY session.
func (c *Client) InteractiveSession(stdin io.Reader, stdout, stderr io.Writer, termWidth, termHeight int) error {
	session, err := c.client.NewSession()
	if err != nil {
		return fmt.Errorf("gagal membuat session: %w", err)
	}
	defer session.Close()

	modes := gossh.TerminalModes{
		gossh.ECHO:          1,
		gossh.TTY_OP_ISPEED: 14400,
		gossh.TTY_OP_OSPEED: 14400,
	}

	if err := session.RequestPty("xterm-256color", termWidth, termHeight, modes); err != nil {
		return fmt.Errorf("gagal request PTY: %w", err)
	}

	session.Stdin = stdin
	session.Stdout = stdout
	session.Stderr = stderr

	if err := session.Shell(); err != nil {
		return fmt.Errorf("gagal memulai shell: %w", err)
	}

	return session.Wait()
}

// Close closes the SSH connection.
func (c *Client) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

func buildSSHConfig(host *Host) (*gossh.ClientConfig, error) {
	var authMethods []gossh.AuthMethod

	// Key-based auth (preferred)
	if host.KeyPath != "" {
		key, err := os.ReadFile(host.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("gagal membaca key file: %w", err)
		}

		signer, err := gossh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("gagal parse private key: %w", err)
		}
		authMethods = append(authMethods, gossh.PublicKeys(signer))
	}

	// Password auth (fallback)
	if host.Password != "" {
		authMethods = append(authMethods, gossh.Password(host.Password))
	}

	if len(authMethods) == 0 {
		return nil, fmt.Errorf("tidak ada metode autentikasi tersedia (key atau password)")
	}

	config := &gossh.ClientConfig{
		User:            host.User,
		Auth:            authMethods,
		HostKeyCallback: gossh.InsecureIgnoreHostKey(), // TODO: store known_hosts
		Timeout:         10 * time.Second,
	}

	return config, nil
}

func validateHost(host *Host) error {
	if host.Address == "" {
		return fmt.Errorf("address host tidak boleh kosong")
	}
	if host.User == "" {
		return fmt.Errorf("username tidak boleh kosong")
	}
	return nil
}
