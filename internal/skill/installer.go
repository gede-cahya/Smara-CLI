package skill

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	installTimeout = 30 * time.Second
	maxSkillSize   = 1 << 20 // 1 MB
)

// InstallOptions configures a remote install.
type InstallOptions struct {
	URL       string // raw URL or GitHub blob/gist shorthand
	Alias     string // optional override for skill name
	Overwrite bool   // allow replacing existing skill
}

// InstallFromURL downloads, validates, and saves a skill from a remote URL.
func InstallFromURL(opts InstallOptions) (*Skill, error) {
	resolved := resolveURL(opts.URL)

	client := &http.Client{Timeout: installTimeout}
	resp, err := client.Get(resolved)
	if err != nil {
		return nil, fmt.Errorf("failed to download skill: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed: %s", resp.Status)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSkillSize))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Try markdown first, then JSON
	var sk *Skill
	if IsMarkdownSkill(data) {
		sk, err = ParseMarkdownSkill(data)
		if err != nil {
			return nil, fmt.Errorf("invalid skill markdown: %w", err)
		}
	} else {
		sk, err = FromJSON(data)
		if err != nil {
			return nil, fmt.Errorf("invalid skill JSON: %w", err)
		}
	}

	if err := sk.Validate(); err != nil {
		return nil, fmt.Errorf("skill validation failed: %w", err)
	}

	// Apply alias if provided
	if opts.Alias != "" {
		sk.Name = opts.Alias
	}

	// Record source URL for future updates
	sk.SourceURL = resolved

	// Check for existing skill
	if existing, _ := Load(sk.Name); existing != nil && !opts.Overwrite {
		return nil, fmt.Errorf("skill '%s' already exists (use --overwrite to replace)", sk.Name)
	}

	// If source is markdown (.md URL or markdown content), save as markdown
	if strings.HasSuffix(resolved, ".md") || IsMarkdownSkill(data) {
		if err := SaveAsMarkdown(sk, nil); err != nil {
			return nil, fmt.Errorf("failed to save skill as markdown: %w", err)
		}
	} else {
		if err := Save(sk, nil); err != nil {
			return nil, fmt.Errorf("failed to save skill: %w", err)
		}
	}

	return sk, nil
}

// resolveURL converts GitHub/gist URLs and shorthands to raw content URLs.
func resolveURL(raw string) string {
	// Already a raw URL?
	if strings.Contains(raw, "raw.githubusercontent.com") || strings.Contains(raw, "gist.githubusercontent.com") {
		return raw
	}

	// GitHub blob URL: github.com/user/repo/blob/branch/path.json
	if strings.Contains(raw, "github.com") && strings.Contains(raw, "/blob/") {
		parts := strings.SplitN(raw, "github.com/", 2)
		if len(parts) == 2 {
			path := parts[1]
			// Remove the "blob/" segment
			path = strings.Replace(path, "/blob/", "/", 1)
			// Convert branch path to raw path
			idx := strings.Index(path, "/")
			if idx > 0 {
				repoEnd := idx
				rest := path[idx+1:]
				// rest now starts with branch/path
				branchIdx := strings.Index(rest, "/")
				if branchIdx >= 0 {
					branch := rest[:branchIdx]
					filePath := rest[branchIdx+1:]
					return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s", path[:repoEnd], branch, filePath)
				}
			}
		}
	}

	// GitHub shorthand without domain: user/repo/path.json
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		parts := strings.Split(raw, "/")
		if len(parts) >= 3 {
			user, repo := parts[0], parts[1]
			path := strings.Join(parts[2:], "/")
			return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/main/%s", user, repo, path)
		}
	}

	// Gist URL: gist.github.com/user/gistid
	if strings.Contains(raw, "gist.github.com") {
		parts := strings.Split(raw, "/")
		if len(parts) >= 2 {
			gistID := parts[len(parts)-1]
			return fmt.Sprintf("https://gist.githubusercontent.com/raw/%s/skill.json", gistID)
		}
	}

	// Direct URL, return as-is
	return raw
}

// UpdateSkill re-downloads a skill from its SourceURL and saves the new version.
func UpdateSkill(name string) (*Skill, error) {
	sk, err := Load(name)
	if err != nil {
		return nil, fmt.Errorf("skill '%s' not found: %w", name, err)
	}
	if sk.SourceURL == "" {
		return nil, fmt.Errorf("skill '%s' has no source URL (was not installed from remote)", name)
	}

	return InstallFromURL(InstallOptions{
		URL:       sk.SourceURL,
		Alias:     name,
		Overwrite: true,
	})
}

// LoadAndValidate is a convenience wrapper that loads a skill and validates it.
func LoadAndValidate(name string) (*Skill, error) {
	sk, err := Load(name)
	if err != nil {
		return nil, err
	}
	if err := sk.Validate(); err != nil {
		return nil, err
	}
	return sk, nil
}

// BackupSkill creates a .bak copy of an existing skill before overwriting.
// Tries .md first (more likely to be human-edited), then .json.
func BackupSkill(name string) error {
	_, err := Load(name)
	if err != nil {
		return nil // nothing to backup
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	// Try markdown first
	src := filepath.Join(home, skillsDir, name+".md")
	dst := filepath.Join(home, skillsDir, name+".md.bak")
	data, err := os.ReadFile(src)
	if err != nil {
		// Fallback to JSON
		src = filepath.Join(home, skillsDir, name+".json")
		dst = filepath.Join(home, skillsDir, name+".json.bak")
		data, err = os.ReadFile(src)
		if err != nil {
			return err
		}
	}
	return os.WriteFile(dst, data, 0644)
}
