package ssh

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Host represents a saved SSH host configuration.
type Host struct {
	Name     string `yaml:"name"`
	Address  string `yaml:"address"`
	Port     string `yaml:"port,omitempty"`
	User     string `yaml:"user"`
	KeyPath  string `yaml:"key_path,omitempty"`
	Password string `yaml:"password,omitempty"`
}

// HostsFile is the top-level YAML structure.
type HostsFile struct {
	Hosts []Host `yaml:"hosts"`
}

// hostsPath returns the path to the hosts YAML file.
func hostsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".smara", "ssh", "hosts.yaml")
}

// EnsureDir ensures the SSH config directory exists.
func EnsureDir() error {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".smara", "ssh")
	return os.MkdirAll(dir, 0700)
}

// LoadHosts reads all saved SSH hosts.
func LoadHosts() ([]Host, error) {
	path := hostsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Host{}, nil
		}
		return nil, fmt.Errorf("gagal membaca hosts file: %w", err)
	}

	var hf HostsFile
	if err := yaml.Unmarshal(data, &hf); err != nil {
		return nil, fmt.Errorf("gagal parse hosts file: %w", err)
	}

	return hf.Hosts, nil
}

// SaveHost adds or updates a host in the config file.
func SaveHost(host Host) error {
	if err := EnsureDir(); err != nil {
		return err
	}

	hosts, err := LoadHosts()
	if err != nil {
		return err
	}

	found := false
	for i, h := range hosts {
		if strings.EqualFold(h.Name, host.Name) {
			hosts[i] = host
			found = true
			break
		}
	}
	if !found {
		hosts = append(hosts, host)
	}

	return writeHosts(hosts)
}

// RemoveHost removes a host by name.
func RemoveHost(name string) error {
	hosts, err := LoadHosts()
	if err != nil {
		return err
	}

	var filtered []Host
	for _, h := range hosts {
		if !strings.EqualFold(h.Name, name) {
			filtered = append(filtered, h)
		}
	}

	return writeHosts(filtered)
}

// GetHost retrieves a host by exact name.
func GetHost(name string) (*Host, error) {
	hosts, err := LoadHosts()
	if err != nil {
		return nil, err
	}

	for _, h := range hosts {
		if strings.EqualFold(h.Name, name) {
			return &h, nil
		}
	}
	return nil, fmt.Errorf("host '%s' tidak ditemukan", name)
}

// FindHost searches for a host by partial/fuzzy name match.
// It returns exact match first, then substring match, then address match.
func FindHost(query string) (*Host, []Host, error) {
	hosts, err := LoadHosts()
	if err != nil {
		return nil, nil, err
	}
	if len(hosts) == 0 {
		return nil, nil, fmt.Errorf("belum ada host SSH tersimpan")
	}

	q := strings.ToLower(query)

	// 1. Exact match
	for _, h := range hosts {
		if strings.EqualFold(h.Name, query) {
			return &h, nil, nil
		}
	}

	// 2. If query contains @, try user@address format
	if strings.Contains(query, "@") {
		for _, h := range hosts {
			ua := h.User + "@" + h.Address
			if strings.EqualFold(ua, query) {
				return &h, nil, nil
			}
		}
	}

	// 3. Substring match on name or address
	var matches []Host
	for _, h := range hosts {
		if strings.Contains(strings.ToLower(h.Name), q) ||
			strings.Contains(strings.ToLower(h.Address), q) ||
			strings.Contains(strings.ToLower(h.User), q) {
			matches = append(matches, h)
		}
	}

	if len(matches) == 1 {
		return &matches[0], nil, nil
	}
	if len(matches) > 1 {
		return nil, matches, fmt.Errorf("multiple hosts cocok dengan '%s'", query)
	}

	return nil, nil, fmt.Errorf("tidak ada host yang cocok dengan '%s'", query)
}

// AllHosts returns all saved hosts as a formatted string for context injection.
func AllHosts() (string, error) {
	hosts, err := LoadHosts()
	if err != nil {
		return "", err
	}
	if len(hosts) == 0 {
		return "(tidak ada host SSH tersimpan)", nil
	}
	var sb strings.Builder
	for _, h := range hosts {
		fmt.Fprintf(&sb, "- %s: %s@%s:%s\n", h.Name, h.User, h.Address, h.Port)
	}
	return sb.String(), nil
}

func writeHosts(hosts []Host) error {
	hf := HostsFile{Hosts: hosts}
	data, err := yaml.Marshal(hf)
	if err != nil {
		return fmt.Errorf("gagal marshal hosts: %w", err)
	}

	path := hostsPath()
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("gagal menulis hosts file: %w", err)
	}
	return nil
}
