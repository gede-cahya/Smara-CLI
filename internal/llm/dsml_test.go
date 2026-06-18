package llm

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractToolCallsFromContent(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectCalls    int
		expectFunc     string
		expectArgs     map[string]string
		expectContains string // content should contain this after cleaning
		expectCleanLen int    // approximate cleaned length
	}{
		{
			name: "ssh_exec DSML block",
			input: `Maaf, SSH tadi belum mengembalikan hasil. Lanjut investigasi.
Sekarang saya lihat isi bridge.js-nya. Fitur "stop membalas" tidak ada di bridge. Pasti ada di gateway atau bot-penunggu. Cek keduanya.
Belum ketemu di bot-penunggu. Cek gateway & struktur direktori hermes.
Ketemu! Ada "/stop" di session.py. Mari lihat detailnya.
"Stop membalas" tidak ada di kode hermes. Cek bot-penunggu - mungkin fiturnya di Discord bot.

<| DSML | tool_calls>
<| DSML | invoke name="ssh_exec">
<| DSML | parameter name="host" string="true">vps-cahya</| DSML | parameter>
<| DSML | parameter name="command" string="true">sed -n '5284,5340p' /home/ubuntu/.hermes/hermes-agent/gateway/run.py</| DSML | parameter>
</| DSML | invoke>
<| DSML | invoke name="ssh_exec">
<| DSML | parameter name="host" string="true">vps-cahya</| DSML | parameter>
<| DSML | parameter name="command" string="true">grep -n "getenv\|COMMAND_PREFIX\|starts\." /home/ubuntu/.hermes/hermes-agent/gateway/whatsapp.py 2>/dev/null | head -20</| DSML | parameter>
</| DSML | invoke>
</| DSML | tool_calls>`,
			expectCalls:    2,
			expectFunc:     "ssh_exec",
			expectArgs:     map[string]string{"host": "vps-cahya"},
			expectContains: "Maaf, SSH tadi belum mengembalikan hasil",
		},
		{
			name:           "no DSML tags",
			input:          "Hello world, no tool calls here.",
			expectCalls:    0,
			expectContains: "Hello world",
		},
		{
			name: "single tool call with multiple params",
			input: `<| DSML | tool_calls>
<| DSML | invoke name="ssh_list_dir">
<| DSML | parameter name="host" string="true">server1</| DSML | parameter>
<| DSML | parameter name="path" string="true">/tmp</| DSML | parameter>
</| DSML | invoke>
</| DSML | tool_calls>`,
			expectCalls: 1,
			expectFunc:  "ssh_list_dir",
			expectArgs:  map[string]string{"host": "server1", "path": "/tmp"},
		},
		{
			name: "double pipe DSML block — regression for raw DSML leak",
			input: `Bot sudah restart. Cek log untuk memastikan tidak ada error:

<｜｜DSML｜｜tool_calls>
<｜｜DSML｜｜invoke name="run_command">
<｜｜DSML｜｜parameter name="command" string="true">pm2 logs bot-penunggu --lines 30 --nostream 2>&1</｜｜DSML｜｜parameter>
</｜｜DSML｜｜invoke>
</｜｜DSML｜｜tool_calls>`,
			expectCalls:    1,
			expectFunc:     "run_command",
			expectArgs:     map[string]string{"command": "pm2 logs bot-penunggu --lines 30 --nostream 2>&1"},
			expectContains: "Bot sudah restart",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls, cleaned := ExtractToolCallsFromContent(tt.input)

			assert.Equal(t, tt.expectCalls, len(calls), "expected %d tool calls", tt.expectCalls)

			if tt.expectCalls > 0 {
				assert.Equal(t, tt.expectFunc, calls[0].Function)
				for k, v := range tt.expectArgs {
					assert.Equal(t, v, calls[0].Args[k])
				}
			}

			if tt.expectContains != "" {
				assert.Contains(t, cleaned, tt.expectContains)
			}

			// Ensure DSML tags are removed
			assert.NotContains(t, cleaned, "<| DSML |")
		})
	}
}

func TestDSMLStreamFilter(t *testing.T) {
	tests := []struct {
		name           string
		chunks         []string
		expectedOutput string
		expectedRemain string // output from Close()
	}{
		{
			name:           "plain text without DSML",
			chunks:         []string{"Hello ", "world", "."},
			expectedOutput: "Hello world.",
		},
		{
			name: "complete DSML block in single chunk",
			chunks: []string{
				"Checking logs...\n<| DSML | tool_calls>\n<| DSML | invoke name=\"run_command\">\n<| DSML | parameter name=\"command\" string=\"true\">uptime</| DSML | parameter>\n</| DSML | invoke>\n</| DSML | tool_calls>",
			},
			expectedOutput: "Checking logs...",
		},
		{
			name: "DSML split across chunks",
			chunks: []string{
				"Here is the result.\n<| DSML | ",
				"tool_calls>\n<| DSML | invoke name=\"run_command\">\n<| DSML | parameter name=\"command\" string=\"true\">uptime</| DSML | parameter>\n</| DSML | invoke>\n</| DSML | tool_calls>",
			},
			expectedOutput: "Here is the result.",
		},
		{
			name: "double pipe DSML block",
			chunks: []string{
				"Done.\n<｜｜DSML｜｜tool_calls>\n<｜｜DSML｜｜invoke name=\"run_command\">\n<｜｜DSML｜｜parameter name=\"command\" string=\"true\">uptime</｜｜DSML｜｜parameter>\n</｜｜DSML｜｜invoke>\n</｜｜DSML｜｜tool_calls>",
			},
			expectedOutput: "Done.",
		},
		{
			name: "angle bracket not DSML — should not hold back forever",
			chunks: []string{
				"Value is < 5 and > 0. ",
				"Next sentence.",
			},
			expectedOutput: "Value is < 5 and > 0. Next sentence.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f DSMLStreamFilter
			var out strings.Builder
			for _, chunk := range tt.chunks {
				out.WriteString(f.Write(chunk))
			}
			assert.Equal(t, tt.expectedOutput, strings.TrimSpace(out.String()))
			// Close should not leave any DSML residue
			remain := f.Close()
			assert.NotContains(t, remain, "DSML")
		})
	}
}

func TestThinkStreamFilter_SingleChunk(t *testing.T) {
	var f ThinkStreamFilter
	content, thinking := f.Write("<think>reasoning here</think>jawaban akhir")
	tailC, tailT := f.Close()
	assert.Equal(t, "jawaban akhir", content+tailC)
	assert.Equal(t, "reasoning here", thinking+tailT)
}

func TestThinkStreamFilter_TagSplitAcrossChunks(t *testing.T) {
	var f ThinkStreamFilter
	var content, thinking strings.Builder
	// "<think>" split as "<thi" + "nk>"; "</think>" split as "</thin" + "k>".
	chunks := []string{"<thi", "nk>be", "rpikir", "</thin", "k>", "Halo dunia"}
	for _, c := range chunks {
		ct, th := f.Write(c)
		content.WriteString(ct)
		thinking.WriteString(th)
	}
	tailC, tailT := f.Close()
	content.WriteString(tailC)
	thinking.WriteString(tailT)
	assert.Equal(t, "Halo dunia", content.String())
	assert.Equal(t, "berpikir", thinking.String())
}

func TestThinkStreamFilter_EmptyThinkBlock(t *testing.T) {
	var f ThinkStreamFilter
	// The exact shard that previously leaked into the live stream.
	content, thinking := f.Write("<think></think>")
	tailC, tailT := f.Close()
	assert.Equal(t, "", content+tailC)
	assert.Equal(t, "", thinking+tailT)
}

func TestThinkStreamFilter_NoThinkTags(t *testing.T) {
	var f ThinkStreamFilter
	content, thinking := f.Write("teks biasa tanpa tag")
	tailC, tailT := f.Close()
	assert.Equal(t, "teks biasa tanpa tag", content+tailC)
	assert.Equal(t, "", thinking+tailT)
}

func TestThinkStreamFilter_ContentBeforeAndAfterThink(t *testing.T) {
	var f ThinkStreamFilter
	var content, thinking strings.Builder
	for _, c := range []string{"awal ", "<think>mid</think>", " akhir"} {
		ct, th := f.Write(c)
		content.WriteString(ct)
		thinking.WriteString(th)
	}
	tailC, tailT := f.Close()
	content.WriteString(tailC)
	thinking.WriteString(tailT)
	assert.Equal(t, "awal  akhir", content.String())
	assert.Equal(t, "mid", thinking.String())
}

func TestThinkStreamFilter_UnclosedThinkAtEnd(t *testing.T) {
	var f ThinkStreamFilter
	content, thinking := f.Write("visible <think>dangling reasoning")
	tailC, tailT := f.Close()
	assert.Equal(t, "visible ", content+tailC)
	assert.Equal(t, "dangling reasoning", thinking+tailT)
}
