package skill

import (
	"database/sql"
	"fmt"
	"time"
)

// Feedback stores user reaction after a skill run.
type Feedback struct {
	ID           int64
	SkillName    string
	RunID        string
	Success      bool
	Notes        string
	ProposedJSON string
	Approved     bool
	CreatedAt    time.Time
}

// SaveFeedback writes feedback to SQLite.
func SaveFeedback(db *sql.DB, f *Feedback) error {
	_, err := db.Exec(`INSERT INTO skill_feedback (skill_name, run_id, success, notes, proposed_json, approved, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		f.SkillName, f.RunID, f.Success, f.Notes, f.ProposedJSON, f.Approved, time.Now())
	return err
}

// GetFeedback retrieves feedback for a skill.
func GetFeedback(db *sql.DB, skillName string, limit int) ([]Feedback, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := db.Query(`SELECT id, skill_name, run_id, success, notes, proposed_json, approved, created_at
		FROM skill_feedback WHERE skill_name = ? ORDER BY created_at DESC LIMIT ?`, skillName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Feedback
	for rows.Next() {
		var f Feedback
		var created time.Time
		err := rows.Scan(&f.ID, &f.SkillName, &f.RunID, &f.Success, &f.Notes, &f.ProposedJSON, &f.Approved, &created)
		if err != nil {
			continue
		}
		f.CreatedAt = created
		list = append(list, f)
	}
	return list, nil
}

// ProposeRefinement generates a new skill JSON based on feedback using LLM.
// This is called by the supervisor after collecting user notes.
func ProposeRefinement(original *Skill, notes string) string {
	// The actual refinement logic is handled by the LLM via system prompt.
	// This function just returns a prompt template for the LLM.
	origJSON, _ := original.ToJSON()
	return fmt.Sprintf(`Kamu adalah Skill Refiner. Skill saat ini:
%s

Feedback user: %s

Buat versi baru skill ini (JSON) yang memperbaiki masalah tersebut. Output HANYA JSON skill, tanpa penjelasan tambahan.`, string(origJSON), notes)
}

// ApplyRefinement overwrites a skill with the proposed version after user approval.
func ApplyRefinement(proposedJSON []byte, db *sql.DB) (*Skill, error) {
	newSkill, err := FromJSON(proposedJSON)
	if err != nil {
		return nil, fmt.Errorf("proposed skill invalid: %w", err)
	}
	newSkill.Version++
	if err := Save(newSkill, db); err != nil {
		return nil, fmt.Errorf("failed to save refined skill: %w", err)
	}
	return newSkill, nil
}

// BuildRefinementPrompt builds the system prompt for the LLM to refine a skill.
func BuildRefinementPrompt(skillName string, db *sql.DB) (string, error) {
	sk, err := Load(skillName)
	if err != nil {
		return "", err
	}
	feedback, err := GetFeedback(db, skillName, 5)
	if err != nil {
		feedback = nil
	}
	origJSON, _ := sk.ToJSON()
	prompt := fmt.Sprintf("Kamu adalah Skill Refiner untuk Smara.\n\nSkill '%s' saat ini:\n%s\n\n",
		sk.Name, string(origJSON))
	if len(feedback) > 0 {
		prompt += "Riwayat feedback:\n"
		for _, f := range feedback {
			status := "gagal"
			if f.Success {
				status = "sukses"
			}
			prompt += fmt.Sprintf("- %s: %s (notes: %s)\n", f.CreatedAt.Format("2006-01-02"), status, f.Notes)
		}
	}
	prompt += "\nUser akan memberikan catatan perbaikan. Buat skill JSON yang lebih baik."
	return prompt, nil
}

// FeedbackFromBool converts a simple success/failure to Feedback.
func FeedbackFromBool(skillName, runID string, success bool) *Feedback {
	return &Feedback{
		SkillName: skillName,
		RunID:     runID,
		Success:   success,
		CreatedAt: time.Now(),
	}
}
