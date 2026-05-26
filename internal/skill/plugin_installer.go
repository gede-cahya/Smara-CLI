package skill

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PluginInstallOptions configures installation from a skill/plugin source.
// A source can be:
//   - a direct URL to a Smara skill JSON/Markdown file
//   - a GitHub shorthand: owner/repo or owner/repo/path/to/skill.json
//   - a local file or directory containing skill manifests
//
// The owner/repo form probes common manifest paths so third-party repos can be
// installed without knowing their exact raw URL.
type PluginInstallOptions struct {
	Source    string
	Alias     string
	Overwrite bool
}

// NormalizePluginSource turns user-facing plugin install forms into the actual
// source understood by the Smara installer. This intentionally treats popular
// external syntax such as `npx skills add owner/repo` as a compatibility alias
// for Smara's safe declarative installer; it does not execute npx.
func NormalizePluginSource(args []string) (string, error) {
	var tokens []string
	for _, arg := range args {
		fields := strings.Fields(strings.TrimSpace(arg))
		if len(fields) == 0 {
			continue
		}
		tokens = append(tokens, fields...)
	}
	if len(tokens) == 0 {
		return "", fmt.Errorf("source is required")
	}
	if len(tokens) >= 4 && (tokens[0] == "npx" || tokens[0] == "bunx" || tokens[0] == "pnpm" || tokens[0] == "pnpx") && tokens[1] == "skills" && tokens[2] == "add" {
		return tokens[3], nil
	}
	if len(tokens) >= 3 && tokens[0] == "skills" && tokens[1] == "add" {
		return tokens[2], nil
	}
	if len(tokens) >= 5 && tokens[0] == "npx" {
		i := 1
		for i < len(tokens) && strings.HasPrefix(tokens[i], "-") {
			i++
		}
		if i+2 < len(tokens) && tokens[i] == "skills" && tokens[i+1] == "add" {
			return tokens[i+2], nil
		}
	}
	if len(tokens) == 1 {
		return tokens[0], nil
	}
	return "", fmt.Errorf("format tidak dikenali; gunakan '<source>' atau 'npx skills add <source>'")
}

// InstallFromPluginSource installs one or more Smara skills from a plugin-like source.
// It intentionally accepts only declarative Smara skill manifests. Arbitrary install
// scripts are not executed here; external plugins must expose a JSON/Markdown skill
// file or a manifest containing a "skills" array.
func InstallFromPluginSource(opts PluginInstallOptions) ([]*Skill, error) {
	source := strings.TrimSpace(opts.Source)
	if source == "" {
		return nil, fmt.Errorf("source is required")
	}
	if isLocalPath(source) {
		return installFromLocalPlugin(source, opts)
	}
	if looksLikeDirectSkillURL(source) || githubShorthandHasPath(source) {
		sk, err := InstallFromURL(InstallOptions{URL: source, Alias: opts.Alias, Overwrite: opts.Overwrite})
		if err != nil {
			return nil, err
		}
		return []*Skill{sk}, nil
	}
	if isGitHubRepoShorthand(source) {
		return installFromGitHubRepo(source, opts)
	}
	sk, err := InstallFromURL(InstallOptions{URL: source, Alias: opts.Alias, Overwrite: opts.Overwrite})
	if err != nil {
		return nil, err
	}
	return []*Skill{sk}, nil
}

func isLocalPath(source string) bool {
	if strings.HasPrefix(source, "file://") {
		return true
	}
	if strings.HasPrefix(source, ".") || strings.HasPrefix(source, "/") || strings.HasPrefix(source, "~") {
		return true
	}
	_, err := os.Stat(expandHome(strings.TrimPrefix(source, "file://")))
	return err == nil
}

func installFromLocalPlugin(source string, opts PluginInstallOptions) ([]*Skill, error) {
	path := expandHome(strings.TrimPrefix(source, "file://"))
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return installSkillsFromBytes(data, path, opts)
	}
	if data, err := os.ReadFile(filepath.Join(path, "SKILL.md")); err == nil {
		sk, err := ParseCodexSkillMarkdown(data, opts.Alias, path)
		if err != nil {
			return nil, err
		}
		if opts.Alias != "" {
			sk.Name = opts.Alias
		}
		sk.SourceURL = path
		if err := saveInstructionSkillFolder(sk, data, path, opts.Overwrite); err != nil {
			return nil, err
		}
		return []*Skill{sk}, nil
	}
	for _, candidate := range pluginManifestCandidates() {
		p := filepath.Join(path, candidate)
		data, err := os.ReadFile(p)
		if err == nil {
			return installSkillsFromBytes(data, p, opts)
		}
	}
	if skills, err := installExternalMarkdownTree(path, opts); err == nil && len(skills) > 0 {
		return skills, nil
	}
	return nil, fmt.Errorf("no skill manifest found in %s", path)
}

func installFromGitHubRepo(source string, opts PluginInstallOptions) ([]*Skill, error) {
	parts := strings.Split(strings.Trim(source, "/"), "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid GitHub repo shorthand: %s", source)
	}
	owner, repo := parts[0], parts[1]
	branches := []string{"main", "master"}
	var tried []string
	var lastErr error
	for _, branch := range branches {
		for _, candidate := range pluginManifestCandidates() {
			url := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s", owner, repo, branch, candidate)
			tried = append(tried, url)
			data, err := fetchLimited(url)
			if err != nil {
				lastErr = err
				continue
			}
			skills, err := installSkillsFromBytes(data, url, opts)
			if err == nil {
				return skills, nil
			}
			lastErr = err
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("no compatible Smara skill manifest found in %s (tried %d paths): %w", source, len(tried), lastErr)
	}
	return nil, fmt.Errorf("no compatible Smara skill manifest found in %s", source)
}

func fetchLimited(url string) ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxSkillSize))
}

func installSkillsFromBytes(data []byte, source string, opts PluginInstallOptions) ([]*Skill, error) {
	if IsMarkdownSkill(data) || strings.HasSuffix(strings.ToLower(source), ".md") {
		sk, err := ParseMarkdownSkill(data)
		markdownIsNative := err == nil
		if err != nil {
			sk, err = ParseExternalInstructionMarkdown(data, opts.Alias, source)
			if err != nil {
				return nil, err
			}
		}
		if opts.Alias != "" {
			sk.Name = opts.Alias
		}
		sk.SourceURL = source
		if markdownIsNative {
			if err := saveInstalledSkill(sk, opts.Overwrite, true); err != nil {
				return nil, err
			}
			return []*Skill{sk}, nil
		}
		if err := saveInstructionSkillFolder(sk, data, "", opts.Overwrite); err != nil {
			return nil, err
		}
		return []*Skill{sk}, nil
	}

	// Single Smara skill JSON.
	if sk, err := FromJSON(data); err == nil && sk.Name != "" && len(sk.Steps) > 0 {
		if opts.Alias != "" {
			sk.Name = opts.Alias
		}
		sk.SourceURL = source
		if err := saveInstalledSkill(sk, opts.Overwrite, false); err != nil {
			return nil, err
		}
		return []*Skill{sk}, nil
	}

	// Manifest with skills array: { "skills": [ {...}, {...} ] }.
	var manifest struct {
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Skills      []Skill `json:"skills"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("invalid skill/plugin manifest JSON: %w", err)
	}
	if len(manifest.Skills) == 0 {
		return nil, fmt.Errorf("manifest does not contain a Smara skill or skills array")
	}
	if opts.Alias != "" && len(manifest.Skills) > 1 {
		return nil, fmt.Errorf("--as/alias can only be used when installing a single skill")
	}
	installed := make([]*Skill, 0, len(manifest.Skills))
	for i := range manifest.Skills {
		sk := manifest.Skills[i]
		if opts.Alias != "" {
			sk.Name = opts.Alias
		}
		sk.SourceURL = source
		if err := saveInstalledSkill(&sk, opts.Overwrite, false); err != nil {
			return nil, err
		}
		installed = append(installed, &sk)
	}
	return installed, nil
}

func installExternalMarkdownTree(root string, opts PluginInstallOptions) ([]*Skill, error) {
	candidates := []string{
		filepath.Join(".claude", "agents"),
		filepath.Join(".claude", "commands"),
		"agents",
		"commands",
		filepath.Join(".antigravity", "agents"),
		filepath.Join(".antigravity", "rules"),
		"rules",
		"skills",
	}
	var files []string
	for _, rel := range candidates {
		dir := filepath.Join(root, rel)
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			continue
		}
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || strings.ToLower(filepath.Ext(path)) != ".md" {
				return nil
			}
			files = append(files, path)
			return nil
		})
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no external markdown agents/commands found")
	}
	if opts.Alias != "" && len(files) > 1 {
		return nil, fmt.Errorf("alias can only be used when installing a single markdown skill")
	}
	installed := make([]*Skill, 0, len(files))
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		sk, err := ParseExternalInstructionMarkdown(data, opts.Alias, file)
		if err != nil {
			return nil, err
		}
		if opts.Alias != "" {
			sk.Name = opts.Alias
		}
		sk.SourceURL = file
		if err := saveInstructionSkillFolder(sk, data, "", opts.Overwrite); err != nil {
			return nil, err
		}
		installed = append(installed, sk)
	}
	return installed, nil
}

func saveInstalledSkill(sk *Skill, overwrite bool, markdown bool) error {
	if err := sk.Validate(); err != nil {
		return err
	}
	if existing, _ := Load(sk.Name); existing != nil && !overwrite {
		return fmt.Errorf("skill '%s' already exists (use --overwrite to replace)", sk.Name)
	}
	if markdown {
		return SaveAsMarkdown(sk, nil)
	}
	return Save(sk, nil)
}

func saveInstructionSkillFolder(sk *Skill, originalMarkdown []byte, sourceDir string, overwrite bool) error {
	if err := sk.Validate(); err != nil {
		return err
	}
	if existing, _ := Load(sk.Name); existing != nil && !overwrite {
		return fmt.Errorf("skill '%s' already exists (use --overwrite to replace)", sk.Name)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dstDir := filepath.Join(home, skillsDir, sk.Name)
	if overwrite {
		_ = os.RemoveAll(dstDir)
	}
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return err
	}
	if sourceDir != "" {
		if err := copySkillDir(sourceDir, dstDir); err != nil {
			return err
		}
	}
	if !bytes.HasPrefix(bytes.TrimSpace(originalMarkdown), []byte("---")) || sourceDir != "" {
		originalMarkdown = renderInstructionSkillMarkdown(sk, skillInstructions(sk, string(originalMarkdown)))
	}
	return os.WriteFile(filepath.Join(dstDir, "SKILL.md"), originalMarkdown, 0644)
}

func copySkillDir(srcDir, dstDir string) error {
	return filepath.WalkDir(srcDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil || rel == "." {
			return err
		}
		dst := filepath.Join(dstDir, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0644)
	})
}

func renderInstructionSkillMarkdown(sk *Skill, body string) []byte {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("name: " + sk.Name + "\n")
	b.WriteString("description: " + strconvQuote(sk.Description) + "\n")
	if sk.Trigger != "" {
		b.WriteString("trigger: " + sk.Trigger + "\n")
	}
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimSpace(body))
	b.WriteString("\n")
	return []byte(b.String())
}

func skillInstructions(sk *Skill, fallback string) string {
	if len(sk.Steps) > 0 {
		if raw, ok := sk.Steps[0].Args["instructions"].(string); ok && strings.TrimSpace(raw) != "" {
			return raw
		}
	}
	return fallback
}

func strconvQuote(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}

func pluginManifestCandidates() []string {
	return []string{
		"smara-skill.json",
		"skill.json",
		"skills.json",
		"smara.skills.json",
		"smara-plugin.json",
		"plugin.json",
		".smara/skill.json",
		".smara/skills.json",
		"skill.md",
		"SKILL.md",
	}
}

func looksLikeDirectSkillURL(source string) bool {
	lower := strings.ToLower(source)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

func githubShorthandHasPath(source string) bool {
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		return false
	}
	parts := strings.Split(strings.Trim(source, "/"), "/")
	return len(parts) >= 3
}

func isGitHubRepoShorthand(source string) bool {
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		return false
	}
	parts := strings.Split(strings.Trim(source, "/"), "/")
	return len(parts) == 2 && parts[0] != "" && parts[1] != ""
}

func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}
