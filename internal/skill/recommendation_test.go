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

func TestRecommendSkillsExactNameBeatsBroadDescription(t *testing.T) {
	skills := []*Skill{
		{Name: "general-deploy-helper", Description: "deploy deploy deploy node aplikasi vps production", Steps: []Step{{Tool: "shell"}}},
		{Name: "deploy-node", Description: "Release app", Steps: []Step{{Tool: "shell"}}},
	}
	recs := RecommendSkills("deploy-node", skills, RecommendationOptions{Limit: 2})
	if len(recs) < 2 || recs[0].SkillName != "deploy-node" {
		t.Fatalf("expected exact name match first, got %#v", recs)
	}
}

func TestRecommendSkillsUsesTriggerAndCategory(t *testing.T) {
	skills := []*Skill{
		{Name: "generic-test", Description: "test helper", Steps: []Step{{Tool: "shell"}}},
		{Name: "planning-risk-review", Description: "review risiko", Trigger: "analisis risiko implementasi", CategoryPath: []string{"planning", "risk"}, Steps: []Step{{Tool: "planning_template"}}},
	}
	recs := RecommendSkills("tolong analisis risiko planning fitur baru", skills, RecommendationOptions{Limit: 2})
	if len(recs) == 0 || recs[0].SkillName != "planning-risk-review" {
		t.Fatalf("expected trigger/category skill first, got %#v", recs)
	}
}

func TestRecommendSkillsPenalizesLowSuccessRate(t *testing.T) {
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
	for i := 0; i < 4; i++ {
		_ = tracker.LogExecution(SkillExecution{SkillName: "deploy-flaky", RunID: string(rune('a' + i)), StartedAt: now, Success: false, DurationMs: 10})
	}
	_ = tracker.LogExecution(SkillExecution{SkillName: "deploy-solid", RunID: "z", StartedAt: now, Success: true, DurationMs: 10})
	skills := []*Skill{
		{Name: "deploy-flaky", Description: "deploy app", Steps: []Step{{Tool: "shell"}}},
		{Name: "deploy-solid", Description: "deploy app", Steps: []Step{{Tool: "shell"}}},
	}
	recs := RecommendSkills("deploy app", skills, RecommendationOptions{StatsProvider: tracker, Limit: 2})
	if len(recs) < 2 || recs[0].SkillName != "deploy-solid" {
		t.Fatalf("expected low success rate penalty, got %#v", recs)
	}
}

func TestRecommendSkillsMarksRecentlyUsed(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tracker, err := NewExecutionTracker(db)
	if err != nil {
		t.Fatal(err)
	}
	_ = tracker.LogExecution(SkillExecution{SkillName: "daily-report", RunID: "1", StartedAt: time.Now(), Success: true, DurationMs: 10})
	skills := []*Skill{{Name: "daily-report", Description: "buat laporan harian", Steps: []Step{{Tool: "shell"}}}}
	recs := RecommendSkills("laporan harian", skills, RecommendationOptions{StatsProvider: tracker})
	if len(recs) == 0 || !recs[0].RecentlyUsed {
		t.Fatalf("expected recently used recommendation, got %#v", recs)
	}
}
