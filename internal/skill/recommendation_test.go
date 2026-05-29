package skill

import (
	"database/sql"
	"testing"
	"time"
)

func TestRecommendSkillsRankingByName(t *testing.T) {
	skills := []*Skill{
		{Name: "monitoring-linux-resources", Description: "Cek resource server", Steps: []Step{{Tool: "shell"}}},
		{Name: "deploy-node-pm2-git-pull", Description: "Deploy aplikasi Node.js", Steps: []Step{{Tool: "ssh_exec"}}},
	}
	recs := RecommendSkills("deploy aplikasi node ke vps", skills, RecommendationOptions{Limit: 2})
	if len(recs) == 0 || recs[0].SkillName != "deploy-node-pm2-git-pull" {
		t.Fatalf("expected deploy skill first, got %#v", recs)
	}
	if len(recs[0].Reasons) == 0 {
		t.Fatal("expected explanation reasons")
	}
}

func TestRecommendSkillsRankingByTag(t *testing.T) {
	skills := []*Skill{
		{Name: "deploy-node", Description: "Release app", Tags: []string{"deploy", "nodejs"}, Steps: []Step{{Tool: "ssh_exec"}}},
		{Name: "database-backup", Description: "Backup db", Tags: []string{"database"}, Steps: []Step{{Tool: "shell"}}},
	}
	recs := RecommendSkills("nodejs", skills, RecommendationOptions{})
	if len(recs) == 0 || recs[0].SkillName != "deploy-node" {
		t.Fatalf("expected tag-matched skill first, got %#v", recs)
	}
}

func TestRecommendSkillsWithSuccessRate(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tracker, err := NewExecutionTracker(db)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	_ = tracker.LogExecution(SkillExecution{SkillName: "deploy-good", RunID: "1", StartedAt: now, Success: true, DurationMs: 10})
	_ = tracker.LogExecution(SkillExecution{SkillName: "deploy-good", RunID: "2", StartedAt: now, Success: true, DurationMs: 10})
	_ = tracker.LogExecution(SkillExecution{SkillName: "deploy-bad", RunID: "3", StartedAt: now, Success: false, DurationMs: 10})
	skills := []*Skill{
		{Name: "deploy-bad", Description: "deploy app", Steps: []Step{{Tool: "shell"}}},
		{Name: "deploy-good", Description: "deploy app", Steps: []Step{{Tool: "shell"}}},
	}
	recs := RecommendSkills("deploy app", skills, RecommendationOptions{StatsProvider: tracker})
	if len(recs) < 2 || recs[0].SkillName != "deploy-good" {
		t.Fatalf("expected higher success rate to rank first, got %#v", recs)
	}
}

func TestRecommendSkillsLowConfidenceClarify(t *testing.T) {
	skills := []*Skill{{Name: "database-backup", Description: "backup database", Steps: []Step{{Tool: "shell"}}}}
	recs := RecommendSkills("xyz obscure", skills, RecommendationOptions{LowConfidence: 25})
	if len(recs) != 0 {
		t.Fatalf("expected no recommendations for unrelated query, got %#v", recs)
	}
	recs = RecommendSkills("data", skills, RecommendationOptions{LowConfidence: 40})
	if len(recs) == 0 || !recs[0].Clarify {
		t.Fatalf("expected low confidence clarify, got %#v", recs)
	}
}
