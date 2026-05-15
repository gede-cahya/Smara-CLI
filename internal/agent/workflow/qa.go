package workflow

import (
	"strings"
)

// QAResult holds the QA review outcome.
type QAResult struct {
	Status string   `json:"status"` // PASS, FAIL, SKIP
	Report string   `json:"report"`
	Issues []string `json:"issues,omitempty"`
	Score  int      `json:"score"` // 0-100
}

// ParseQAResult parses LLM output into a structured QAResult.
func ParseQAResult(output string) QAResult {
	output = strings.TrimSpace(output)
	qr := QAResult{
		Status: "PENDING",
		Report: output,
		Score:  0,
	}

	lower := strings.ToLower(output)
	if strings.Contains(lower, "status: pass") || strings.Contains(lower, "pass") && !strings.Contains(lower, "fail") {
		qr.Status = "PASS"
		qr.Score = 95
	} else if strings.Contains(lower, "status: fail") || strings.Contains(lower, "fail") {
		qr.Status = "FAIL"
		qr.Score = 30
	}

	// Extract issues
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- ") && (strings.Contains(line, "issue") || strings.Contains(line, "conflict") || strings.Contains(line, "error") || strings.Contains(line, "mismatch")) {
			qr.Issues = append(qr.Issues, strings.TrimPrefix(line, "- "))
		}
	}

	return qr
}

// QASystemPrompt is the system prompt for the QA reviewer agent.
const QASystemPrompt = `Kamu adalah QA/Reviewer Agent. Tugasmu memeriksa integrasi hasil semua agen.

1. Bandingkan hasil kerja semua agen dengan PRD asli
2. Cek apakah API contract dari Backend cocok dengan data fetching di Frontend
3. Cek apakah schema DB sesuai dengan endpoint API
4. Cek apakah desain UI sesuai dengan PRD
5. Laporkan PASS atau FAIL dengan detail

Format laporan (WAJIB):
Status: PASS / FAIL
Score: [0-100]
Issues: (list jika ada)
- [issue 1]
- [issue 2]
Rekomendasi: (saran perbaikan)`
