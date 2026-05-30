package platform

import (
	"testing"

	"github.com/gede-cahya/Smara-CLI/internal/agent"
)

func TestShouldAutoParallelOrchestrate(t *testing.T) {
	cases := []struct {
		name   string
		prompt string
		mode   agent.Mode
		want   bool
	}{
		{
			name:   "workflow mode substantial prompt stays normal without explicit parallel",
			prompt: "tolong audit repo ini lalu build dan test semua komponen penting",
			mode:   agent.ModeWorkflow,
			want:   false,
		},
		{
			name:   "complex multi step adapter prompt stays normal without explicit parallel",
			prompt: "tolong cek repo project ini, perbaiki bug build, jalankan test, lalu update docs dan verifikasi hasilnya",
			mode:   agent.ModeRush,
			want:   false,
		},
		{
			name:   "ordinary chat stays supervisor",
			prompt: "apa kabar dan jelaskan singkat tentang go routine",
			mode:   agent.ModeAsk,
			want:   false,
		},
		{
			name:   "workflow mode greeting stays supervisor",
			prompt: "hallo",
			mode:   agent.ModeWorkflow,
			want:   false,
		},
		{
			name:   "workflow mode short question stays supervisor",
			prompt: "apa kabar",
			mode:   agent.ModeWorkflow,
			want:   false,
		},
		{
			name:   "explicit parallel work outside parallel mode stays normal",
			prompt: "jalankan parallel orchestration untuk audit repo dan build test",
			mode:   agent.ModeAsk,
			want:   false,
		},
		{
			name:   "explicit parallel work routes only in parallel mode",
			prompt: "jalankan parallel orchestration untuk audit repo dan build test",
			mode:   agent.ModeParallel,
			want:   true,
		},
		{
			name:   "opt out",
			prompt: "tolong audit repo dan build test tapi jangan paralel",
			mode:   agent.ModeWorkflow,
			want:   false,
		},
		{
			name:   "follow up question does not route to orchestration",
			prompt: "tadi saya suruh jalankan workflow github release agent kok belum ada perubahan github saya yang saya jalankan secara parallel",
			mode:   agent.ModeAsk,
			want:   false,
		},
		{
			name:   "custom workflow desire does not auto route",
			prompt: "saya mau bisa juga untuk parallel untuk menjalankan workflow custom workflow yang saya buat",
			mode:   agent.ModeAsk,
			want:   false,
		},
		{
			name:   "custom workflow parallel orchestration mention stays custom router",
			prompt: "tolong di perbaiki saya mau jalankan custom workflow aja untuk parallel orchestrasion ketika di panggil atau di pakai",
			mode:   agent.ModeRush,
			want:   false,
		},
		{
			name:   "agent swarm explicit run outside parallel mode stays normal",
			prompt: "jalankan Agent Swarm Workflow untuk audit repo ini, pecah tugas, spawn agent coder tester reviewer lalu build",
			mode:   agent.ModeRush,
			want:   false,
		},
		{
			name:   "agent swarm explicit run routes only in parallel mode",
			prompt: "jalankan Agent Swarm Workflow untuk audit repo ini, pecah tugas, spawn agent coder tester reviewer lalu build",
			mode:   agent.ModeParallel,
			want:   true,
		},
		{
			name:   "agent swarm question stays chat",
			prompt: "apakah smara bisa melakukan Agent Swarm Workflow?",
			mode:   agent.ModeAsk,
			want:   false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldAutoParallelOrchestrate(tc.prompt, tc.mode)
			if got != tc.want {
				t.Fatalf("shouldAutoParallelOrchestrate()=%v want %v", got, tc.want)
			}
		})
	}
}
