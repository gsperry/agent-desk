package main

import (
	"os"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("../../internal/pane/testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestCheck(t *testing.T) {
	tests := []struct {
		name, status, screen string
		flagged              bool
	}{
		{"idle prompt", "idle", fixture(t, "idle.txt"), false},
		{"permission", "waiting", fixture(t, "permission.txt"), false},
		{"multi-select", "waiting", fixture(t, "askq-multi-initial.txt"), false},
		{"review step", "waiting", fixture(t, "askq-review.txt"), false},
		// A question layout the parser does not know: a hint line, but the
		// options drawn with a marker it does not read.
		{"unknown question", "waiting", "Pick one?\n> Red\n  Blue\nEnter to select · Esc to cancel\n", true},
		{"waiting with nothing", "waiting", "some output\n", true},
		{"idle without a prompt box", "idle", "some output\n", true},
	}
	for _, tt := range tests {
		problems, summary := check(tt.status, tt.screen)
		if (len(problems) > 0) != tt.flagged {
			t.Errorf("%s: problems %v (summary %q)", tt.name, problems, summary)
		}
		if strings.Contains(summary, "Red") {
			t.Errorf("%s: summary leaks content", tt.name)
		}
	}
}
