package platform

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSanitizeDSML_EditFileLeak regresses the Telegram leak where a response
// like "PM2 sebenarnya jalan! Hanya issue PATH di script. Biar saya perbaiki."
// was sent with raw DSML <｜｜DSML｜｜tool_calls>...<｜｜DSML｜｜invoke name="edit_file">
// appended. sanitizeDSML must remove all such markup while preserving the
// prose.
func TestSanitizeDSML_EditFileLeak(t *testing.T) {
	raw := `PM2 sebenarnya jalan! Hanya issue PATH di script. Biar saya perbaiki.

<｜｜DSML｜｜tool_calls>
<｜｜DSML｜｜invoke name="edit_file">
<｜｜DSML｜｜parameter name="path" string="true">cek-program.sh</｜｜DSML｜｜parameter>
<｜｜DSML｜｜parameter name="old_string" string="true">#!/bin/bash
# =============================================
# SKILL: cek-program
# Quick overview semua service di VPS
# =============================================

clear</｜｜DSML｜｜parameter>
<｜｜DSML｜｜parameter name="new_string" string="true">#!/bin/bash
# =============================================
# SKILL: cek-program
# Quick overview semua service di VPS
# =============================================

export PATH=/home/ubuntu/.bun/bin:$PATH
export HOME=/home/ubuntu</｜｜DSML｜｜parameter>
</｜｜DSML｜｜invoke>
</｜｜DSML｜｜tool_calls>`

	cleaned := sanitizeDSML(raw)

	assert.Contains(t, cleaned, "PM2 sebenarnya jalan!")
	assert.Contains(t, cleaned, "Hanya issue PATH di script")

	// Absolutely no DSML residue should remain in any known variant.
	assert.NotContains(t, cleaned, "DSML")
	assert.NotContains(t, cleaned, "tool_calls")
	assert.NotContains(t, cleaned, "invoke name=")
	assert.NotContains(t, cleaned, "｜")
}

// TestSanitizeDSML_Empty ensures empty input returns empty output.
func TestSanitizeDSML_Empty(t *testing.T) {
	assert.Equal(t, "", sanitizeDSML(""))
}

// TestSanitizeDSML_NoDSML ensures clean text passes through unchanged (after trim).
func TestSanitizeDSML_NoDSML(t *testing.T) {
	in := "Halo, ini jawaban biasa tanpa DSML."
	out := sanitizeDSML(in)
	assert.Equal(t, strings.TrimSpace(in), out)
}

// TestSanitizeDSML_AsciiPipes ensures the common ASCII-pipe variant is also
// stripped (some models normalize away the fullwidth pipes).
func TestSanitizeDSML_AsciiPipes(t *testing.T) {
	raw := `Selesai cek log.

<| DSML | tool_calls>
<| DSML | invoke name="run_command">
<| DSML | parameter name="command" string="true">uptime</| DSML | parameter>
</| DSML | invoke>
</| DSML | tool_calls>`

	cleaned := sanitizeDSML(raw)
	assert.Contains(t, cleaned, "Selesai cek log.")
	assert.NotContains(t, cleaned, "DSML")
}

// TestSanitizeDSML_SkillRunLeak regresses the user-reported bug where
// "tadi response kamu <|DSML|invoke name="skill_run"> <|DSML|parameter name="skill_name">auto-cek-status-vps</|DSML|parameter> </|DSML|invoke>Skill tolong di fix"
// leaked raw DSML to Telegram/Web/Discord surfaces.
func TestSanitizeDSML_SkillRunLeak(t *testing.T) {
	cases := []string{
		`Berhasil! Berikut status VPS kamu:

| Metrik | Status |
|---|---|
| **Uptime** | 21 hari |

<|DSML|invoke name="skill_run"> <|DSML|parameter name="skill_name">auto-cek-status-vps</|DSML|parameter> </|DSML|invoke>Skill  tolong di fix dulu di smara web, smara adapter telegram , discord`,
		"Selesai cek.\n\n<｜｜DSML｜｜tool_calls>\n<｜｜DSML｜｜invoke name=\"skill_run\">\n<｜｜DSML｜｜parameter name=\"skill_name\" string=\"true\">auto-cek-status-vps</｜｜DSML｜｜parameter>\n</｜｜DSML｜｜invoke>\n</｜｜DSML｜｜tool_calls>",
		"PM2 sebenarnya jalan! Hanya issue PATH di script. Biar saya perbaiki. <|DSML|invoke name=\"skill_run\">",
		"Oke, saya lanjutkan eksekusi, tapi harus saya hentikan di tengah karena batas iterasi tool tercapai.\n\ntadi response kamu <|DSML|invoke name=\"skill_run\"> <|DSML|parameter name=\"skill_name\">auto-cek-status-vps</|DSML|parameter> </|DSML|invoke>Skill  tolong di fix dulu di smara web, smara adapter telegram , discord",
	}
	for i, raw := range cases {
		cleaned := sanitizeDSML(raw)
		assert.NotContains(t, cleaned, "DSML", "case %d should not contain DSML", i+1)
		assert.NotContains(t, cleaned, "invoke name=", "case %d should not contain invoke", i+1)
		assert.NotContains(t, cleaned, "｜", "case %d should not contain fullwidth pipe", i+1)
		// skill_name inside DSML must be stripped; plain mention of skill name in user prose is ok only if DSML was present — here DSML present so must strip
		if strings.Contains(raw, "<|DSML|") || strings.Contains(raw, "｜｜DSML｜｜") {
			assert.NotContains(t, cleaned, "auto-cek-status-vps", "case %d skill_name leak", i+1)
		}
	}
}

// TestSanitizeDSML_PartialTruncated ensures truncated DSML at end of answer
// (common in max-iterations fallback) is removed.
func TestSanitizeDSML_PartialTruncated(t *testing.T) {
	raw := "Jawaban setengah jadi <|DSML|invoke name=\"skill_run\"> <|DSML|parameter name=\"skill_name\">"
	cleaned := sanitizeDSML(raw)
	assert.NotContains(t, cleaned, "DSML")
	assert.Contains(t, cleaned, "Jawaban setengah jadi")
}
