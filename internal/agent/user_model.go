package agent

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// UserProfile stores adaptive preferences inferred from user behavior.
type UserProfile struct {
	ID                 int64
	Verbosity          string `json:"verbosity"`
	RiskTolerance      string `json:"risk_tolerance"`
	PrimaryDomains     string `json:"primary_domains"`
	PreferredLanguages string `json:"preferred_languages"`
	CustomPatterns     string `json:"custom_patterns"`
	SessionCount       int    `json:"session_count"`
	TotalPrompts       int    `json:"total_prompts"`
	LastActive         *time.Time
	UpdatedAt          time.Time
}

// DefaultProfile returns a new profile with sensible defaults.
func DefaultProfile() *UserProfile {
	now := time.Now()
	return &UserProfile{
		Verbosity:          "balanced",
		RiskTolerance:      "balanced",
		PrimaryDomains:     "[]",
		PreferredLanguages: "[]",
		CustomPatterns:     "{}",
		SessionCount:       0,
		TotalPrompts:       0,
		UpdatedAt:          now,
	}
}

// LoadProfile retrieves the user profile from DB (singleton row with id=1).
func LoadProfile(db *sql.DB) (*UserProfile, error) {
	var p UserProfile
	var lastActive sql.NullTime
	err := db.QueryRow(`SELECT id, verbosity, risk_tolerance, primary_domains,
		preferred_languages, custom_patterns, session_count, total_prompts,
		last_active, updated_at FROM user_profile WHERE id = 1`).Scan(
		&p.ID, &p.Verbosity, &p.RiskTolerance, &p.PrimaryDomains,
		&p.PreferredLanguages, &p.CustomPatterns, &p.SessionCount, &p.TotalPrompts,
		&lastActive, &p.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		p = *DefaultProfile()
		p.ID = 1
		_, err2 := db.Exec(`INSERT INTO user_profile (id, verbosity, risk_tolerance, primary_domains,
			preferred_languages, custom_patterns, session_count, total_prompts, updated_at)
			VALUES (1, 'balanced', 'balanced', '[]', '[]', '{}', 0, 0, ?)`, p.UpdatedAt)
		if err2 != nil {
			return nil, fmt.Errorf("gagal membuat default user_profile: %w", err2)
		}
		return &p, nil
	}
	if err != nil {
		return nil, err
	}
	if lastActive.Valid {
		p.LastActive = &lastActive.Time
	}
	return &p, nil
}

// SaveProfile writes the profile back to DB.
func SaveProfile(db *sql.DB, p *UserProfile) error {
	p.UpdatedAt = time.Now()
	_, err := db.Exec(`UPDATE user_profile SET
		verbosity = ?, risk_tolerance = ?, primary_domains = ?,
		preferred_languages = ?, custom_patterns = ?,
		session_count = ?, total_prompts = ?, last_active = ?, updated_at = ?
		WHERE id = 1`,
		p.Verbosity, p.RiskTolerance, p.PrimaryDomains,
		p.PreferredLanguages, p.CustomPatterns,
		p.SessionCount, p.TotalPrompts, p.LastActive, p.UpdatedAt)
	return err
}

// RecordSession increments session count and updates last_active.
func RecordSession(db *sql.DB) error {
	_, err := db.Exec(`UPDATE user_profile SET
		session_count = session_count + 1, last_active = ?, updated_at = ?
		WHERE id = 1`, time.Now(), time.Now())
	return err
}

// RecordPrompt increments total_prompts.
func RecordPrompt(db *sql.DB) error {
	_, err := db.Exec(`UPDATE user_profile SET
		total_prompts = total_prompts + 1, updated_at = ?
		WHERE id = 1`, time.Now())
	return err
}

// UpdateFromPreference applies a user-stated preference to the profile.
func UpdateFromPreference(db *sql.DB, key, value string) error {
	p, err := LoadProfile(db)
	if err != nil {
		return err
	}
	switch key {
	case "verbosity":
		p.Verbosity = value
	case "risk_tolerance":
		p.RiskTolerance = value
	case "primary_domains", "domains":
		p.PrimaryDomains = value
	case "preferred_languages", "languages":
		p.PreferredLanguages = value
	case "custom_patterns", "patterns":
		p.CustomPatterns = value
	}
	return SaveProfile(db, p)
}

// ToContext returns a short string to inject into the system prompt.
func (p *UserProfile) ToContext() string {
	var parts []string
	parts = append(parts, fmt.Sprintf("Verbosity: %s", p.Verbosity))
	parts = append(parts, fmt.Sprintf("Risk tolerance: %s", p.RiskTolerance))

	var domains []string
	json.Unmarshal([]byte(p.PrimaryDomains), &domains)
	if len(domains) > 0 {
		parts = append(parts, fmt.Sprintf("Primary domains: %s", strings.Join(domains, ", ")))
	}

	var langs []string
	json.Unmarshal([]byte(p.PreferredLanguages), &langs)
	if len(langs) > 0 {
		parts = append(parts, fmt.Sprintf("Preferred languages: %s", strings.Join(langs, ", ")))
	}

	var patterns map[string]interface{}
	if err := json.Unmarshal([]byte(p.CustomPatterns), &patterns); err == nil && len(patterns) > 0 {
		parts = append(parts, "Custom patterns:")
		for k, v := range patterns {
			parts = append(parts, fmt.Sprintf("  - %s: %v", k, v))
		}
	}

	return "User profile:\n" + strings.Join(parts, "\n")
}
