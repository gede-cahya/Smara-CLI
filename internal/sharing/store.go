package sharing

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gede-cahya/Smara-CLI/internal/config"
)

type Visibility string

const (
	VisibilityPrivate   Visibility = "private"
	VisibilityWorkspace Visibility = "workspace"
	VisibilityTeam      Visibility = "team"
)

type Resource struct {
	Type       string     `json:"type"`
	Name       string     `json:"name"`
	Visibility Visibility `json:"visibility"`
	Team       string     `json:"team,omitempty"`
	Workspace  string     `json:"workspace,omitempty"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type Store struct {
	Version   int        `json:"version"`
	Resources []Resource `json:"resources"`
	UpdatedAt time.Time  `json:"updated_at"`
}

func Load() (*Store, error) {
	data, err := os.ReadFile(Path())
	if os.IsNotExist(err) {
		return &Store{Version: 1, Resources: []Resource{}, UpdatedAt: time.Now()}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("gagal baca sharing metadata: %w", err)
	}
	var store Store
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, fmt.Errorf("gagal parse sharing metadata: %w", err)
	}
	if store.Version == 0 {
		store.Version = 1
	}
	return &store, nil
}

func Save(store *Store) error {
	if err := os.MkdirAll(filepath.Dir(Path()), 0755); err != nil {
		return err
	}
	store.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("gagal marshal sharing metadata: %w", err)
	}
	return os.WriteFile(Path(), data, 0644)
}

func Set(resourceType, name string, visibility Visibility, team string) (*Resource, error) {
	if err := validateVisibility(visibility); err != nil {
		return nil, err
	}
	store, err := Load()
	if err != nil {
		return nil, err
	}
	resourceType = strings.ToLower(strings.TrimSpace(resourceType))
	name = strings.TrimSpace(name)
	if resourceType == "" || name == "" {
		return nil, fmt.Errorf("type dan name wajib diisi")
	}
	cfg := config.Get()
	res := Resource{Type: resourceType, Name: name, Visibility: visibility, Team: team, Workspace: cfg.ActiveWorkspace, UpdatedAt: time.Now()}
	for i := range store.Resources {
		if store.Resources[i].Type == resourceType && store.Resources[i].Name == name {
			store.Resources[i] = res
			return &store.Resources[i], Save(store)
		}
	}
	store.Resources = append(store.Resources, res)
	return &store.Resources[len(store.Resources)-1], Save(store)
}

func Get(resourceType, name string) (*Resource, error) {
	store, err := Load()
	if err != nil {
		return nil, err
	}
	resourceType = strings.ToLower(strings.TrimSpace(resourceType))
	name = strings.TrimSpace(name)
	for i := range store.Resources {
		if store.Resources[i].Type == resourceType && store.Resources[i].Name == name {
			return &store.Resources[i], nil
		}
	}
	return &Resource{Type: resourceType, Name: name, Visibility: VisibilityPrivate, Workspace: config.Get().ActiveWorkspace}, nil
}

func List(resourceType string) ([]Resource, error) {
	store, err := Load()
	if err != nil {
		return nil, err
	}
	resourceType = strings.ToLower(strings.TrimSpace(resourceType))
	var resources []Resource
	for _, res := range store.Resources {
		if resourceType == "" || res.Type == resourceType {
			resources = append(resources, res)
		}
	}
	sort.Slice(resources, func(i, j int) bool {
		if resources[i].Type == resources[j].Type {
			return resources[i].Name < resources[j].Name
		}
		return resources[i].Type < resources[j].Type
	})
	return resources, nil
}

func Path() string {
	cfg := config.Get()
	return filepath.Join(filepath.Dir(cfg.DBPath), "sharing", "metadata.json")
}

func validateVisibility(v Visibility) error {
	switch v {
	case VisibilityPrivate, VisibilityWorkspace, VisibilityTeam:
		return nil
	default:
		return fmt.Errorf("visibility harus private, workspace, atau team")
	}
}
