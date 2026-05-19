package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParsePlanQuest(t *testing.T) {
	input := `Sebelum lanjut, pilih dulu.

[[SMARA_PLAN_QUEST]]
title: Bagian mana yang diprioritaskan?
options:
- Web dulu
- CLI dulu
- Web dan CLI
allow_custom: true
[[/SMARA_PLAN_QUEST]]`

	clean, quest := ParsePlanQuest(input)

	assert.Equal(t, "Sebelum lanjut, pilih dulu.", clean)
	assert.NotNil(t, quest)
	assert.Equal(t, "Bagian mana yang diprioritaskan?", quest.Title)
	assert.Equal(t, []string{"Web dulu", "CLI dulu", "Web dan CLI"}, quest.Options)
	assert.True(t, quest.AllowCustom)
}

func TestParsePlanQuest_NoMarker(t *testing.T) {
	input := "Pesan biasa"
	clean, quest := ParsePlanQuest(input)
	assert.Equal(t, input, clean)
	assert.Nil(t, quest)
}

func TestParsePlanQuest_IncompleteMarker(t *testing.T) {
	input := "[[SMARA_PLAN_QUEST]]\ntitle: Test"
	clean, quest := ParsePlanQuest(input)
	assert.Equal(t, input, clean)
	assert.Nil(t, quest)
}
