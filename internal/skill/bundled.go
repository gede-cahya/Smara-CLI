package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// BundledSkillItem is the metadata shown for a bundled skill before install.
type BundledSkillItem struct {
	Name         string     `json:"name"`
	Description  string     `json:"description"`
	Version      int        `json:"version"`
	Tags         []string   `json:"tags"`
	Params       []ParamDef `json:"params,omitempty"`
	CategoryPath []string   `json:"category_path,omitempty"`
	Dependencies []string   `json:"dependencies,omitempty"`
}

// FindBundledSkillsDir locates the repository or installed bundled skills directory.
func FindBundledSkillsDir() string {
	candidates := []string{"skills"}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "skills"))
	}
	if _, file, _, ok := runtime.Caller(0); ok {
		repoRoot := filepath.Join(filepath.Dir(file), "..", "..")
		candidates = append(candidates, filepath.Join(repoRoot, "skills"))
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return candidate
		}
	}
	return ""
}

// ListBundledSkills returns all valid bundled JSON skills.
func ListBundledSkills() ([]BundledSkillItem, error) {
	dir := FindBundledSkillsDir()
	if dir == "" {
		return []BundledSkillItem{}, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	items := make([]BundledSkillItem, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		sk, err := FromJSON(data)
		if err != nil || sk.Name == "" {
			continue
		}
		items = append(items, BundledSkillItem{
			Name:         sk.Name,
			Description:  sk.Description,
			Version:      sk.Version,
			Tags:         append([]string(nil), sk.Tags...),
			Params:       append([]ParamDef(nil), sk.Params...),
			CategoryPath: append([]string(nil), sk.CategoryPath...),
			Dependencies: append([]string(nil), sk.Dependencies...),
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, nil
}

// InstallBundledSkill installs a bundled skill into the user's skill store.
func InstallBundledSkill(name, alias string, overwrite bool) (*Skill, error) {
	if !validBundledSkillName(name) {
		return nil, fmt.Errorf("invalid bundled skill name: %s", name)
	}
	dir := FindBundledSkillsDir()
	if dir == "" {
		return nil, fmt.Errorf("bundled skills directory not found")
	}
	data, err := os.ReadFile(filepath.Join(dir, name+".json"))
	if err != nil {
		return nil, fmt.Errorf("bundled skill '%s' not found: %w", name, err)
	}
	sk, err := FromJSON(data)
	if err != nil {
		return nil, fmt.Errorf("invalid bundled skill JSON: %w", err)
	}
	if alias != "" {
		sk.Name = alias
	}
	if !overwrite {
		if _, err := Load(sk.Name); err == nil {
			return nil, fmt.Errorf("skill '%s' already exists (use --overwrite to replace)", sk.Name)
		}
	}
	if err := Save(sk, nil); err != nil {
		return nil, err
	}
	return sk, nil
}

func validBundledSkillName(name string) bool {
	if name == "" || strings.Contains(name, "..") {
		return false
	}
	return !strings.ContainsAny(name, `/\\`)
}
