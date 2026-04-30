package llm

import (
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
		expectCleanLen int  // approximate cleaned length
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
