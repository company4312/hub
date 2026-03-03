package agent

import (
	"testing"
)

func TestMemoryMarkerRe(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantCount int
		wantCat   string
		wantSrc   string
		wantBody  string
	}{
		{
			name:      "single marker",
			input:     `Hello [MEMORY category="decision" source="chat"]use postgres[/MEMORY] world`,
			wantCount: 1,
			wantCat:   "decision",
			wantSrc:   "chat",
			wantBody:  "use postgres",
		},
		{
			name:      "multiline content",
			input:     "[MEMORY category=\"lesson_learned\" source=\"review\"]line1\nline2[/MEMORY]",
			wantCount: 1,
			wantCat:   "lesson_learned",
			wantSrc:   "review",
			wantBody:  "line1\nline2",
		},
		{
			name:      "no markers",
			input:     "just a regular response",
			wantCount: 0,
		},
		{
			name:      "two markers",
			input:     `[MEMORY category="skill" source="a"]one[/MEMORY] mid [MEMORY category="context" source="b"]two[/MEMORY]`,
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := memoryMarkerRe.FindAllStringSubmatch(tt.input, -1)
			if len(matches) != tt.wantCount {
				t.Fatalf("got %d matches, want %d", len(matches), tt.wantCount)
			}
			if tt.wantCount > 0 && tt.wantCat != "" {
				if matches[0][1] != tt.wantCat {
					t.Errorf("category = %q, want %q", matches[0][1], tt.wantCat)
				}
				if matches[0][2] != tt.wantSrc {
					t.Errorf("source = %q, want %q", matches[0][2], tt.wantSrc)
				}
				if matches[0][3] != tt.wantBody {
					t.Errorf("body = %q, want %q", matches[0][3], tt.wantBody)
				}
			}
		})
	}
}

func TestMemoryMarkerStripping(t *testing.T) {
	input := `Here is my answer. [MEMORY category="decision" source="chat"]use postgres[/MEMORY] Done.`
	want := "Here is my answer.  Done."
	got := memoryMarkerRe.ReplaceAllString(input, "")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
