package agent

import (
	"testing"

	"github.com/gede-cahya/Smara-CLI/internal/llm"
)

func TestIsDirectImageGenerationRequestSkipsSoftwareFeatureRequests(t *testing.T) {
	cases := []string{
		"buatkan fitur image to image nya",
		"implement image to image",
		"tambah fitur upload gambar",
		"buatkan component image editor",
	}
	for _, tc := range cases {
		if isDirectImageGenerationRequest(tc) {
			t.Fatalf("expected %q not to route to direct image generation", tc)
		}
	}
}

func TestIsDirectImageGenerationRequestAllowsVisualGeneration(t *testing.T) {
	cases := []string{
		"buatkan logo smara",
		"buatkan gambar kucing lucu",
		"generate poster event",
	}
	for _, tc := range cases {
		if !isDirectImageGenerationRequest(tc) {
			t.Fatalf("expected %q to route to direct image generation", tc)
		}
	}
}

func TestFilterToolsForPromptIntentRemovesImageToolForSoftwareFeature(t *testing.T) {
	tools := []llm.ToolFunction{{Name: "generate_image"}, {Name: "write_file"}}
	filtered := filterToolsForPromptIntent(tools, "buatkan fitur image to image untuk smara web", ModeImage)
	for _, tool := range filtered {
		if tool.Name == "generate_image" {
			t.Fatalf("expected generate_image to be hidden for software image feature prompts")
		}
	}
	if len(filtered) != 1 || filtered[0].Name != "write_file" {
		t.Fatalf("unexpected filtered tools: %#v", filtered)
	}
}

func TestFilterToolsForPromptIntentRemovesImageToolOutsideImageMode(t *testing.T) {
	tools := []llm.ToolFunction{{Name: "generate_image"}, {Name: "write_file"}}
	filtered := filterToolsForPromptIntent(tools, "buatkan logo smara", ModeAsk)
	for _, tool := range filtered {
		if tool.Name == "generate_image" {
			t.Fatalf("expected generate_image to be hidden outside image mode")
		}
	}
	if len(filtered) != 1 || filtered[0].Name != "write_file" {
		t.Fatalf("unexpected filtered tools: %#v", filtered)
	}
}

func TestFilterToolsForPromptIntentKeepsImageToolForVisualRequestInImageMode(t *testing.T) {
	tools := []llm.ToolFunction{{Name: "generate_image"}, {Name: "write_file"}}
	filtered := filterToolsForPromptIntent(tools, "buatkan logo smara", ModeImage)
	if len(filtered) != len(tools) {
		t.Fatalf("expected visual requests in image mode to keep all tools, got %#v", filtered)
	}
}

func TestFilterToolsDynamicFiltering(t *testing.T) {
	tools := []llm.ToolFunction{
		{Name: "write_file"},              // core
		{Name: "skill_list"},              // reusable skill core
		{Name: "skill_run"},               // reusable skill core
		{Name: "ssh_exec"},                // ssh
		{Name: "lsp_hover"},               // lsp
		{Name: "dbsnp_lookup"},            // science MCP (auto-detected via name)
		{Name: "chrome_lighthouse_audit"}, // chrome MCP (auto-detected via name)
		{Name: "custom_db_query", Description: "run query on custom sql database"}, // other/custom
	}

	// Case 1: General greeting/question - only core tools kept (others filtered)
	{
		filtered := filterToolsForPromptIntent(tools, "halo, apa kabar?", ModeAsk)
		hasCore := false
		for _, tool := range filtered {
			if tool.Name == "write_file" || tool.Name == "skill_list" || tool.Name == "skill_run" {
				hasCore = true
			}
			if tool.Name != "write_file" && tool.Name != "skill_list" && tool.Name != "skill_run" {
				t.Fatalf("expected non-core tool %q to be filtered for general prompt, got it", tool.Name)
			}
		}
		if !hasCore {
			t.Fatalf("expected core tool 'write_file' to be kept")
		}
	}

	// Case 2: SSH prompt - ssh kept, others filtered
	{
		filtered := filterToolsForPromptIntent(tools, "restart server via ssh", ModeAsk)
		hasSSH := false
		hasCore := false
		for _, tool := range filtered {
			if tool.Name == "write_file" {
				hasCore = true
			}
			if tool.Name == "ssh_exec" {
				hasSSH = true
			}
			if tool.Name != "write_file" && tool.Name != "skill_list" && tool.Name != "skill_run" && tool.Name != "ssh_exec" {
				t.Fatalf("expected tool %q to be filtered for SSH prompt", tool.Name)
			}
		}
		if !hasCore || !hasSSH {
			t.Fatalf("expected core and ssh tools to be kept, got: %#v", filtered)
		}
	}

	// Case 3: Science prompt - science MCP tool kept, others filtered
	{
		filtered := filterToolsForPromptIntent(tools, "cari info gene uniprot", ModeAsk)
		hasScience := false
		for _, tool := range filtered {
			if tool.Name == "dbsnp_lookup" {
				hasScience = true
			}
			if tool.Name == "ssh_exec" || tool.Name == "lsp_hover" || tool.Name == "chrome_lighthouse_audit" {
				t.Fatalf("expected tool %q to be filtered for science prompt", tool.Name)
			}
		}
		if !hasScience {
			t.Fatalf("expected science tool 'dbsnp_lookup' to be kept")
		}
	}

	// Case 4: Other/Custom tool fallback match by name
	{
		filtered := filterToolsForPromptIntent(tools, "tolong run query di db", ModeAsk)
		hasCustom := false
		for _, tool := range filtered {
			if tool.Name == "custom_db_query" {
				hasCustom = true
			}
		}
		if !hasCustom {
			t.Fatalf("expected custom tool 'custom_db_query' to be kept via generic keyword matching")
		}
	}

	// Case 5: Code & Image Explanation prompts bypass tools completely for maximum speed
	{
		codePrompt := "if __name__ == \"__main__\":\n    main() ini script apa"
		filtered := filterToolsForPromptIntent(tools, codePrompt, ModeAsk)
		if len(filtered) != 0 {
			t.Fatalf("expected 0 tools for code explanation prompt %q, got: %#v", codePrompt, filtered)
		}

		imgPrompt := "[image:/tmp/clip.png] ini gambar apa"
		filteredImg := filterToolsForPromptIntent(tools, imgPrompt, ModeAsk)
		if len(filteredImg) != 0 {
			t.Fatalf("expected 0 tools for image explanation prompt %q, got: %#v", imgPrompt, filteredImg)
		}

		contPrompt := "lanjutkan"
		filteredCont := filterToolsForPromptIntent(tools, contPrompt, ModeAsk)
		if len(filteredCont) != 0 {
			t.Fatalf("expected 0 tools for continuation prompt %q, got: %#v", contPrompt, filteredCont)
		}
	}
}
