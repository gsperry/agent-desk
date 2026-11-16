package pane

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gsperry/agent-desk/internal/hub"
	"github.com/gsperry/agent-desk/internal/send"
)

// listScreen draws the bottom of a Claude Code screen with its agent list,
// in the shape the probe recorded: the prompt between two rules (titled
// while a subagent is viewed), the hint line, then main and each subagent.
// cursor is -1 when not selecting; shown is the row the screen shows.
func listScreen(texts []string, cursor, shown int, title, prompt string) string {
	return listScreenHint(texts, cursor, shown, title, prompt, "")
}

// listScreenHint is listScreen with the hint line given, such as the one
// offering x on a running subagent.
func listScreenHint(texts []string, cursor, shown int, title, prompt, hintLine string) string {
	rule := strings.Repeat("─", 99)
	top := rule
	if title != "" {
		top = strings.Repeat("─", 90) + " " + title + " ─"
	}
	box := "❯ "
	switch {
	case prompt != "":
		box += prompt
	case shown > 0:
		box += "Message @general-purpose…"
	}
	hint := "⏸ manual mode on · ? for shortcuts · ↓ to manage"
	if cursor >= 0 {
		hint = "↑/↓ to select"
	}
	if hintLine != "" {
		hint = hintLine
	}
	lines := []string{"⏺ Working on it.", "", top, box, rule, "  " + hint, ""}
	rows := append([]string{"main"}, texts...)
	for i, r := range rows {
		mark := "◯"
		if i == shown {
			mark = "⏺"
		}
		lead := "  "
		if i == cursor {
			lead = "❯ "
		}
		if i == 0 {
			lines = append(lines, lead+mark+" main")
		} else {
			lines = append(lines, fmt.Sprintf("%s%s general-purpose  %-50s  9s · ↓ 15.0k tokens", lead, mark, r))
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func TestListScreenParses(t *testing.T) {
	l := ParseAgents(listScreen([]string{"Pinger"}, 1, 1, "Pinger", ""))
	if l == nil || !l.Viewing || l.Title != "Pinger" || l.Cursor != 1 || l.Shown != 1 || !l.PromptEmpty || l.Rows[1].Text != "Pinger" {
		t.Fatalf("list = %+v", l)
	}
}

func TestMessageAgent(t *testing.T) {
	one := []string{"Pinger"}
	busy := []string{"Running Bash command.", "Running Bash command."}
	pinger := AgentTarget{Type: "general-purpose", Description: "Pinger"}
	// The way back to main from a subagent's conversation, after typing.
	back := func(texts []string, row int, title string) []string {
		return []string{
			listScreen(texts, -1, row, title, ""), // restoreMain reads
			listScreen(texts, -1, row, title, ""), // selectRow reads, then Down
			listScreen(texts, 0, row, title, ""),  // the list opens on main; Enter
			listScreen(texts, 0, 0, "", ""),       // main again; Up closes the list
			listScreen(texts, -1, 0, "", ""),
		}
	}
	happy := append([]string{
		listScreen(one, -1, 0, "", ""), // MessageAgent reads
		listScreen(one, -1, 0, "", ""), // selectRow reads, then Down
		listScreen(one, 0, 0, "", ""),  // list open on main; Down
		listScreen(one, 1, 0, "", ""),  // on Pinger; Enter
		listScreen(one, 1, 1, "Pinger", ""),
		listScreen(one, -1, 1, "Pinger", ""), // after typing: still Pinger, prompt empty
	}, back(one, 1, "Pinger")...)
	// Two rows show what they are doing, not their descriptions: the
	// first viewed is another subagent, so Agent Desk tries the next.
	second := append([]string{
		listScreen(busy, -1, 0, "", ""),
		listScreen(busy, -1, 0, "", ""),
		listScreen(busy, 0, 0, "", ""),
		listScreen(busy, 1, 0, "", ""),
		listScreen(busy, 1, 1, "Stopper", ""), // not it
		listScreen(busy, 1, 1, "Stopper", ""), // selectRow reads: still selecting on row 1; Down
		listScreen(busy, 2, 1, "Stopper", ""), // Enter
		listScreen(busy, 2, 2, "Pinger", ""),
		listScreen(busy, -1, 2, "Pinger", ""),
	}, back(busy, 2, "Pinger")...)

	tests := []struct {
		name     string
		screens  []string
		target   AgentTarget
		text     string
		wantKeys []string
		typed    []string
		reason   send.Reason
	}{
		{"messages the subagent and comes back to main", happy, pinger, " Also say hello. ", []string{"Down", "Down", "Enter", "Down", "Enter", "Up"}, []string{"Also say hello."}, ""},
		{"finds the subagent by its description", second, pinger, "hi", []string{"Down", "Down", "Enter", "Down", "Enter", "Down", "Enter", "Up"}, []string{"hi"}, ""},
		{"a dialog on screen", []string{read(t, "permission.txt")}, pinger, "hi", nil, nil, reasonAsking},
		{"no agent list", []string{read(t, "idle.txt"), read(t, "idle.txt")}, pinger, "hi", nil, nil, reasonNoList},
		{"text in the prompt", []string{listScreen(one, -1, 0, "", "half a thought")}, pinger, "hi", nil, nil, send.ReasonDraft},
		{"another type", []string{listScreen(one, -1, 0, "", "")}, AgentTarget{Type: "Explore", Description: "Pinger"}, "hi", nil, nil, reasonNotListed},
		{"an empty message", []string{listScreen(one, -1, 0, "", "")}, pinger, "  ", nil, nil, send.ReasonEmpty},
		{"Enter does not show it", []string{
			listScreen(one, -1, 0, "", ""), listScreen(one, -1, 0, "", ""), listScreen(one, 0, 0, "", ""), listScreen(one, 1, 0, "", ""),
			listScreen(one, 1, 0, "", ""), // Enter changed nothing
			listScreen(one, 1, 0, "", ""), // restoreMain: main shown, list open; Up
			listScreen(one, 1, 0, "", ""), listScreen(one, 0, 0, "", ""), listScreen(one, -1, 0, "", ""),
		}, pinger, "hi", []string{"Down", "Down", "Enter", "Up", "Up"}, nil, reasonLost},
		{"the text stays in the box", []string{
			listScreen(one, -1, 0, "", ""), listScreen(one, -1, 0, "", ""), listScreen(one, 0, 0, "", ""), listScreen(one, 1, 0, "", ""),
			listScreen(one, 1, 1, "Pinger", ""),
			listScreen(one, -1, 1, "Pinger", "hi"), // not sent
			listScreen(one, -1, 1, "Pinger", "hi"), // restoreMain: text in the box stops it
		}, pinger, "hi", []string{"Down", "Down", "Enter"}, []string{"hi"}, reasonLost},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys := &recordKeys{}
			typer := &recordTyper{}
			a := &Answerer{
				Sessions: &fakeSessions{st: hub.State{Sessions: []hub.SessionView{{ID: "s1", Running: true, Status: "working"}}}},
				Reader:   &seqReader{screens: tt.screens},
				Keys:     keys,
				Typer:    typer,
			}
			err := a.MessageAgent(context.Background(), "s1", tt.target, tt.text)
			if tt.reason != "" {
				var refused *send.Refused
				if !errors.As(err, &refused) || refused.Reason != tt.reason {
					t.Fatalf("err = %v, want %s", err, tt.reason)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			// Escape interrupts a viewed subagent, so the flow never presses it.
			for _, k := range keys.got {
				if k == "Escape" {
					t.Fatal("pressed Escape")
				}
			}
			if !reflect.DeepEqual(keys.got, tt.wantKeys) || !reflect.DeepEqual(typer.got, tt.typed) {
				t.Fatalf("keys = %v typed = %q; want %v %q", keys.got, typer.got, tt.wantKeys, tt.typed)
			}
			if r := tt.screens; tt.reason == "" {
				if n := a.Reader.(*seqReader).n; n != len(r) {
					t.Errorf("read %d screens, script has %d", n, len(r))
				}
			}
		})
	}
}

func TestStopAgent(t *testing.T) {
	one := []string{"Pinger"}
	pinger := AgentTarget{Type: "general-purpose", Description: "Pinger"}
	stop := "Enter to view · x to stop · ctrl+x ctrl+k to stop all agents"
	cleared := "Enter to view · x to clear"
	// Confirm the row by showing it, then back to main.
	confirm := []string{
		listScreen(one, -1, 0, "", ""),      // StopAgent reads
		listScreen(one, -1, 0, "", ""),      // selectRow reads; Down
		listScreen(one, 0, 0, "", ""),       // Down
		listScreen(one, 1, 0, "", ""),       // Enter
		listScreen(one, 1, 1, "Pinger", ""), // it is Pinger
		listScreen(one, 1, 1, "Pinger", ""), // restoreMain reads
		listScreen(one, 1, 1, "Pinger", ""), // selectRow reads; Up
		listScreen(one, 0, 1, "Pinger", ""), // Enter on main
		listScreen(one, 0, 0, "", ""),       // Up closes the list
		listScreen(one, -1, 0, "", ""),
	}
	// From main, select the row again; the hint decides.
	reselect := func(hint string) []string {
		return []string{
			listScreen(one, -1, 0, "", ""), // selectRow reads; Down
			listScreen(one, 0, 0, "", ""),  // Down
			listScreenHint(one, 1, 0, "", "", hint),
		}
	}
	closeList := []string{
		listScreenHint(one, 1, 0, "", "", cleared), // restoreMain reads
		listScreenHint(one, 1, 0, "", "", cleared), // selectRow reads; Up
		listScreen(one, 0, 0, "", ""),              // Up closes the list
		listScreen(one, -1, 0, "", ""),
	}
	cat := func(parts ...[]string) []string {
		var out []string
		for _, p := range parts {
			out = append(out, p...)
		}
		return out
	}
	tests := []struct {
		name     string
		screens  []string
		wantKeys []string
		reason   send.Reason
	}{
		{"stops the subagent", cat(confirm, reselect(stop), []string{listScreenHint(one, 1, 0, "", "", cleared)}, closeList),
			[]string{"Down", "Down", "Enter", "Up", "Enter", "Up", "Down", "Down", "x", "Up", "Up"}, ""},
		{"already stopped", cat(confirm, reselect(cleared), closeList),
			[]string{"Down", "Down", "Enter", "Up", "Enter", "Up", "Down", "Down", "Up", "Up"}, reasonNotListed},
		{"x changes nothing", cat(confirm, reselect(stop), []string{listScreenHint(one, 1, 0, "", "", stop)}, []string{
			listScreenHint(one, 1, 0, "", "", stop), listScreenHint(one, 1, 0, "", "", stop), listScreen(one, 0, 0, "", ""), listScreen(one, -1, 0, "", ""),
		}), []string{"Down", "Down", "Enter", "Up", "Enter", "Up", "Down", "Down", "x", "Up", "Up"}, reasonLost},
		{"not listed", []string{listScreen(one, -1, 0, "", "")}, nil, reasonNotListed},
		{"a dialog on screen", []string{read(t, "permission.txt")}, nil, reasonAsking},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys := &recordKeys{}
			a := &Answerer{
				Sessions: &fakeSessions{st: hub.State{Sessions: []hub.SessionView{{ID: "s1", Running: true, Status: "working"}}}},
				Reader:   &seqReader{screens: tt.screens},
				Keys:     keys,
			}
			target := pinger
			if tt.name == "not listed" {
				target = AgentTarget{Type: "Explore", Description: "Pinger"}
			}
			err := a.StopAgent(context.Background(), "s1", target)
			if tt.reason != "" {
				var refused *send.Refused
				if !errors.As(err, &refused) || refused.Reason != tt.reason {
					t.Fatalf("err = %v, want %s", err, tt.reason)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			for _, k := range keys.got {
				if k == "Escape" {
					t.Fatal("pressed Escape")
				}
			}
			if !reflect.DeepEqual(keys.got, tt.wantKeys) {
				t.Fatalf("keys = %v, want %v", keys.got, tt.wantKeys)
			}
		})
	}
}

// x reaches the pane only through StopAgent: a key answer refuses it.
func TestStopKeyIsNotAKeyAnswer(t *testing.T) {
	keys := &recordKeys{}
	a := &Answerer{
		Sessions: &fakeSessions{st: hub.State{Sessions: []hub.SessionView{{ID: "s1", Running: true}}}},
		Reader:   fakeReader{screen: read(t, "permission.txt")},
		Keys:     keys,
	}
	var refused *send.Refused
	if err := a.Answer(context.Background(), "s1", Answer{Action: "key", Key: "x"}); !errors.As(err, &refused) || refused.Reason != reasonBadKey || len(keys.got) != 0 {
		t.Fatalf("err = %v keys = %v", err, keys.got)
	}
}
