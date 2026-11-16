package pane

import (
	"reflect"
	"testing"
)

func TestParseAgents(t *testing.T) {
	pinger := AgentRow{Type: "general-purpose", Text: "Pinger"}
	tests := []struct {
		file      string
		rows      []AgentRow
		hint      string
		selecting bool
		cursor    int
		shown     int
		viewing   bool
		title     string
		empty     bool
	}{
		{"agents-main.txt", []AgentRow{{Main: true}, pinger}, "⏸ manual mode on · ? for shortcuts · ↓ to manage", false, -1, 0, false, "", true},
		{"agents-select-main.txt", []AgentRow{{Main: true}, pinger}, "↑/↓ to select", true, 0, 0, false, "", true},
		{"agents-select-running.txt", []AgentRow{{Main: true}, pinger, {Type: "general-purpose", Text: "Stopper"}}, "Enter to view · x to stop · ctrl+x ctrl+k to stop all agents", true, 2, 0, false, "", true},
		{"agents-viewing.txt", []AgentRow{{Main: true}, pinger}, "↑/↓ to select", true, 1, 1, true, "Pinger", true},
		{"agents-viewing-queued.txt", []AgentRow{{Main: true}, {Type: "general-purpose", Text: "Pinger", Queued: 1}}, "⏸ manual mode on · ? for shortcuts · ↓ to manage", false, -1, 1, true, "Pinger", true},
		{"agents-select-stopped.txt", []AgentRow{{Main: true}, pinger, {Type: "general-purpose", Text: "Stopper"}}, "Enter to view · x to clear", true, 2, 0, false, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			l := ParseAgents(read(t, tt.file))
			if l == nil {
				t.Fatal("no agent list")
			}
			for i := range l.Rows {
				l.Rows[i].Stats = ""
			}
			if !reflect.DeepEqual(l.Rows, tt.rows) {
				t.Errorf("rows = %+v, want %+v", l.Rows, tt.rows)
			}
			if l.Hint != tt.hint || l.Selecting != tt.selecting || l.Cursor != tt.cursor || l.Shown != tt.shown || l.Viewing != tt.viewing || l.Title != tt.title || l.PromptEmpty != tt.empty {
				t.Errorf("list = %+v", *l)
			}
		})
	}
}

func TestParseAgentsAbsent(t *testing.T) {
	for _, f := range []string{"idle.txt", "permission.txt", "askq-tabs.txt", "askq-3tabs-review.txt"} {
		if l := ParseAgents(read(t, f)); l != nil {
			t.Errorf("%s: list = %+v", f, *l)
		}
	}
}

func TestAgentCandidates(t *testing.T) {
	l := ParseAgents(read(t, "agents-select-running.txt"))
	if got := l.candidates("general-purpose", "Stopper"); !reflect.DeepEqual(got, []int{2, 1}) {
		t.Errorf("candidates = %v", got)
	}
	if got := l.candidates("plugin:general-purpose", "Missing"); !reflect.DeepEqual(got, []int{1, 2}) {
		t.Errorf("plugin type = %v", got)
	}
	if got := l.candidates("Explore", "Pinger"); got != nil {
		t.Errorf("other type = %v", got)
	}
}
