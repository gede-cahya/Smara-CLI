package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"

	gossh "golang.org/x/crypto/ssh"
)

// GenerateKeyPair creates an SSH key pair and saves it to ~/.smara/ssh/keys/.
func GenerateKeyPair(name, keyType string, bits int) (pubPath, privPath string, err error) {
	if err := EnsureDir(); err != nil {
		return "", "", err
	}

	home, _ := os.UserHomeDir()
	keysDir := filepath.Join(home, ".smara", "ssh", "keys")
	if err := os.MkdirAll(keysDir, 0700); err != nil {
		return "", "", fmt.Errorf("gagal membuat keys dir: %w", err)
	}

	privPath = filepath.Join(keysDir, name)
	pubPath = privPath + ".pub"

	switch keyType {
	case "ed25519":
		pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return "", "", fmt.Errorf("gagal generate ed25519 key: %w", err)
		}

		privBytes, err := x509.MarshalPKCS8PrivateKey(privKey)
		if err != nil {
			return "", "", fmt.Errorf("gagal marshal private key: %w", err)
		}

		privBlock := &pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}
		if err := os.WriteFile(privPath, pem.EncodeToMemory(privBlock), 0600); err != nil {
			return "", "", fmt.Errorf("gagal menulis private key: %w", err)
		}

		sshPubKey, err := gossh.NewPublicKey(pubKey)
		if err != nil {
			return "", "", fmt.Errorf("gagal membuat SSH public key: %w", err)
		}

		pubContent := gossh.MarshalAuthorizedKey(sshPubKey)
		if err := os.WriteFile(pubPath, pubContent, 0644); err != nil {
			return "", "", fmt.Errorf("gagal menulis public key: %w", err)
		}

	case "rsa":
		if bits == 0 {
			bits = 4096
		}
		privKey, err := rsa.GenerateKey(rand.Reader, bits)
		if err != nil {
			return "", "", fmt.Errorf("gagal generate RSA key: %w", err)
		}

		privBlock := &pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(privKey),
		}
		if err := os.WriteFile(privPath, pem.EncodeToMemory(privBlock), 0600); err != nil {
			return "", "", fmt.Errorf("gagal menulis private key: %w", err)
		}

		pubKey, err := gossh.NewPublicKey(&privKey.PublicKey)
		if err != nil {
			return "", "", fmt.Errorf("gagal membuat SSH public key: %w", err)
		}

		pubContent := gossh.MarshalAuthorizedKey(pubKey)
		if err := os.WriteFile(pubPath, pubContent, 0644); err != nil {
			return "", "", fmt.Errorf("gagal menulis public key: %w", err)
		}

	default:
		return "", "", fmt.Errorf("tipe key tidak didukung: %s (pilih: ed25519, rsa)", keyType)
	}

	return pubPath, privPath, nil
}
