package skill

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// CompareChange describes one field-level difference between two skill versions.
type CompareChange struct {
	Field string      `json:"field"`
	From  interface{} `json:"from,omitempty"`
	To    interface{} `json:"to,omitempty"`
}

// CompareResult is the structured result of comparing two skill snapshots.
type CompareResult struct {
	Name        string          `json:"name"`
	FromVersion int             `json:"from_version"`
	ToVersion   int             `json:"to_version"`
	Changes     []CompareChange `json:"changes"`
}

func History(name string) ([]LineageEntry, *Skill, error) {
	s, err := Load(name)
	if err != nil {
		return nil, nil, err
	}
	return append([]LineageEntry(nil), s.Lineage...), s, nil
}

// ResolveVersion accepts "v2" or "2" and returns the numeric version.
func ResolveVersion(raw string) (int, error) {
	raw = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(raw), "v"))
	if raw == "" {
		return 0, fmt.Errorf("version is required")
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("invalid version %q", raw)
	}
	return v, nil
}

// SnapshotVersion returns the skill snapshot for a version. Current version is supported too.
func SnapshotVersion(current *Skill, version int) (*Skill, error) {
	if current == nil {
		return nil, fmt.Errorf("skill is nil")
	}
	if current.Version == version {
		return current, nil
	}
	for _, e := range current.Lineage {
		if e.Version == version {
			if strings.TrimSpace(e.Snapshot) == "" {
				return nil, fmt.Errorf("version %d has no snapshot", version)
			}
			s, err := FromJSON([]byte(e.Snapshot))
			if err != nil {
				return nil, err
			}
			return s, nil
		}
	}
	return nil, fmt.Errorf("version %d not found in lineage", version)
}

// CompareVersions compares two stored versions of a skill.
func CompareVersions(name string, fromVersion, toVersion int) (*CompareResult, error) {
	current, err := Load(name)
	if err != nil {
		return nil, err
	}
	from, err := SnapshotVersion(current, fromVersion)
	if err != nil {
		return nil, err
	}
	to, err := SnapshotVersion(current, toVersion)
	if err != nil {
		return nil, err
	}
	res := &CompareResult{Name: current.Name, FromVersion: fromVersion, ToVersion: toVersion}
	add := func(field string, a, b interface{}) {
		if !reflect.DeepEqual(a, b) {
			res.Changes = append(res.Changes, CompareChange{Field: field, From: a, To: b})
		}
	}
	add("description", from.Description, to.Description)
	add("tags", from.Tags, to.Tags)
	add("author", from.Author, to.Author)
	add("source_url", from.SourceURL, to.SourceURL)
	add("trigger", from.Trigger, to.Trigger)
	add("params", from.Params, to.Params)
	add("dependencies", from.Dependencies, to.Dependencies)
	add("steps", from.Steps, to.Steps)
	return res, nil
}

func Rollback(name string, version int, db *sql.DB) (*Skill, error) {
	current, err := Load(name)
	if err != nil {
		return nil, err
	}
	prior, err := SnapshotVersion(current, version)
	if err != nil {
		return nil, err
	}
	if prior == current {
		return nil, fmt.Errorf("cannot rollback to current version %d", version)
	}
	prior.Name = current.Name
	AttachLineage(prior, current, fmt.Sprintf("rollback-to-v%d", version))
	if prior.Version <= current.Version {
		prior.Version = current.Version + 1
	}
	if err := Save(prior, db); err != nil {
		return nil, err
	}
	return prior, nil
}

func (r CompareResult) JSON() string {
	data, _ := json.MarshalIndent(r, "", "  ")
	return string(data)
}
