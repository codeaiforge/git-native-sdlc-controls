package core

import "testing"

func TestParseProvenance(t *testing.T) {
	tests := []struct {
		name     string
		messages []string
		want     Provenance
	}{
		{
			name:     "no trailers",
			messages: []string{"fix: correct the rounding on settlement\n"},
			want:     Provenance{},
		},
		{
			name: "full trailer block; a value may itself contain a colon",
			messages: []string{"feat: add tier policy loader\n\n" +
				"AI-Assisted: true\nAI-Tool: claude-code\n" +
				"AI-Session: 4f2c1a80-7d3e-4b19-9c55-0ae61b2d8f34\n" +
				"Prompt-Ref: https://example.com/issues/12\n"},
			want: Provenance{
				AIAssisted: true,
				AITool:     "claude-code",
				AISession:  "4f2c1a80-7d3e-4b19-9c55-0ae61b2d8f34",
				PromptRef:  "https://example.com/issues/12",
			},
		},
		{
			name:     "keys are case-insensitive and values are trimmed",
			messages: []string{"chore: tidy\n\nai-assisted:   yes  \nAI-TOOL:  cursor \n"},
			want:     Provenance{AIAssisted: true, AITool: "cursor"},
		},
		{
			name:     "AI-Assisted false stays false",
			messages: []string{"chore: tidy\n\nAI-Assisted: false\n"},
			want:     Provenance{},
		},
		{
			name:     "a named tool implies assistance even without the flag",
			messages: []string{"chore: tidy\n\nAI-Tool: claude-code\n"},
			want:     Provenance{AIAssisted: true, AITool: "claude-code"},
		},
		{
			name: "one AI-assisted commit marks the whole change",
			messages: []string{
				"fix: manual tweak\n",
				"feat: generated adapter\n\nAI-Assisted: true\nAI-Tool: claude-code\n",
			},
			want: Provenance{AIAssisted: true, AITool: "claude-code"},
		},
		{
			name: "the most recent tool wins",
			messages: []string{
				"a\n\nAI-Assisted: true\nAI-Tool: cursor\n",
				"b\n\nAI-Assisted: true\nAI-Tool: claude-code\n",
			},
			want: Provenance{AIAssisted: true, AITool: "claude-code"},
		},
		{
			name: "several sessions record none of them, but still mark the change",
			messages: []string{
				"a\n\nAI-Assisted: true\nAI-Tool: claude-code\nAI-Session: one\n",
				"b\n\nAI-Assisted: true\nAI-Tool: claude-code\nAI-Session: two\n",
			},
			want: Provenance{AIAssisted: true, AITool: "claude-code"},
		},
		{
			name: "one session repeated across commits is still one session",
			messages: []string{
				"a\n\nAI-Assisted: true\nAI-Tool: claude-code\nAI-Session: one\n",
				"b\n\nAI-Assisted: true\nAI-Tool: claude-code\nAI-Session: one\n",
			},
			want: Provenance{AIAssisted: true, AITool: "claude-code", AISession: "one"},
		},
		{
			name:     "a session alone still implies assistance when there are several",
			messages: []string{"a\n\nAI-Session: one\n", "b\n\nAI-Session: two\n"},
			want:     Provenance{AIAssisted: true},
		},
		{
			name:     "unrelated trailers are ignored",
			messages: []string{"fix: thing\n\nSigned-off-by: A Dev <a@example.com>\nCo-Authored-By: B Dev <b@example.com>\n"},
			want:     Provenance{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseProvenance(tc.messages); got != tc.want {
				t.Errorf("ParseProvenance() = %+v, want %+v", got, tc.want)
			}
		})
	}
}
