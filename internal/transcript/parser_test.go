package transcript

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func parseOne(t *testing.T, line string) []Item {
	t.Helper()
	return NewParser().Line([]byte(line))
}

func TestLine(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		kinds []Kind
		check func(t *testing.T, items []Item)
	}{
		{
			name:  "typed prompt",
			line:  `{"type":"user","uuid":"u1","promptSource":"typed","origin":{"kind":"human"},"message":{"role":"user","content":"Fix the bug"}}`,
			kinds: []Kind{KindUser},
			check: func(t *testing.T, it []Item) {
				if it[0].Text != "Fix the bug" || it[0].Meta["source"] != "typed" {
					t.Fatalf("%+v", it[0])
				}
			},
		},
		{
			name:  "agent-manager preamble is split off",
			line:  `{"type":"user","uuid":"u2","message":{"content":"This session is already named. You can rename it later with agent-manager rename \"<name>\" only if the user asks. Do not rename it now. Then do the task:\n\nShip it."}}`,
			kinds: []Kind{KindUser},
			check: func(t *testing.T, it []Item) {
				if it[0].Text != "Ship it." || !strings.HasSuffix(it[0].Preamble, "Then do the task:") {
					t.Fatalf("text %q preamble %q", it[0].Text, it[0].Preamble)
				}
			},
		},
		{
			name:  "deferred rename directive",
			line:  `{"type":"user","uuid":"u3","message":{"content":"Run this exact shell command once: agent-manager rename \"<name>\". Run rename only this once. Then continue."}}`,
			kinds: []Kind{KindNotice},
			check: labelIs("agent-manager"),
		},
		{
			name:  "slash command",
			line:  `{"type":"user","uuid":"u4","message":{"content":"<command-message>review</command-message>\n<command-name>/review</command-name>\n<command-args>PR 12</command-args>"}}`,
			kinds: []Kind{KindNotice},
			check: func(t *testing.T, it []Item) {
				if it[0].Label != "command" || it[0].Text != "/review PR 12" {
					t.Fatalf("%+v", it[0])
				}
			},
		},
		{
			name:  "command output with ANSI",
			line:  `{"type":"user","uuid":"u5","message":{"content":"<local-command-stdout>\u001b[1mDone\u001b[0m</local-command-stdout>"}}`,
			kinds: []Kind{KindNotice},
			check: func(t *testing.T, it []Item) {
				if it[0].Label != "command-output" || it[0].Text != "Done" {
					t.Fatalf("%+v", it[0])
				}
			},
		},
		{
			name:  "task notification by origin",
			line:  `{"type":"user","uuid":"u6","origin":{"kind":"task-notification"},"message":{"content":"<task-notification>done</task-notification>"}}`,
			kinds: []Kind{KindNotice},
			check: labelIs("task-notification"),
		},
		{
			name:  "peer message",
			line:  `{"type":"user","uuid":"u7","isMeta":true,"origin":{"kind":"peer"},"message":{"content":"From api-session: ready"}}`,
			kinds: []Kind{KindPeer},
		},
		{
			name:  "system reminder",
			line:  `{"type":"user","uuid":"u8","isMeta":true,"message":{"content":"<system-reminder>be brief</system-reminder>"}}`,
			kinds: []Kind{KindNotice},
			check: labelIs("system-reminder"),
		},
		{
			name:  "other meta text",
			line:  `{"type":"user","uuid":"u9","isMeta":true,"message":{"content":"Caveat text"}}`,
			kinds: []Kind{KindNotice},
			check: labelIs("injected"),
		},
		{
			name:  "interrupt marker",
			line:  `{"type":"user","uuid":"u10","message":{"content":[{"type":"text","text":"[Request interrupted by user]"}]}}`,
			kinds: []Kind{KindNotice},
			check: labelIs("interrupted"),
		},
		{
			name:  "compact summary",
			line:  `{"type":"user","uuid":"u11","isCompactSummary":true,"message":{"content":"Summary of earlier work"}}`,
			kinds: []Kind{KindCompactSummary},
		},
		{
			name:  "tool result with structured edit detail",
			line:  `{"type":"user","uuid":"u12","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"ok"}]},"toolUseResult":{"filePath":"/a.go","originalFile":"WHOLE FILE","structuredPatch":[{"oldStart":1,"lines":["-a","+b"]}]}}`,
			kinds: []Kind{KindToolResult},
			check: func(t *testing.T, it []Item) {
				r := it[0].Result
				if r.ToolUseID != "toolu_1" || r.Content != "ok" {
					t.Fatalf("%+v", r)
				}
				d := string(r.Detail)
				if strings.Contains(d, "WHOLE FILE") || !strings.Contains(d, "structuredPatch") {
					t.Fatalf("detail %s", d)
				}
			},
		},
		{
			name:  "tool result content blocks and error",
			line:  `{"type":"user","uuid":"u13","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_2","is_error":true,"content":[{"type":"text","text":"line one"},{"type":"image","source":{"media_type":"image/png"}}]}]}}`,
			kinds: []Kind{KindToolResult},
			check: func(t *testing.T, it []Item) {
				if r := it[0].Result; !r.IsError || r.Content != "line one\n[image]" {
					t.Fatalf("%+v", r)
				}
			},
		},
		{
			name:  "pasted image",
			line:  `{"type":"user","uuid":"u14","message":{"content":[{"type":"text","text":"see this"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AAAA"}}]}}`,
			kinds: []Kind{KindUser, KindImage},
		},
		{
			name:  "assistant blocks share the message id",
			line:  `{"type":"assistant","uuid":"a1","thinkingDurationMs":1200,"message":{"id":"msg_1","content":[{"type":"thinking","thinking":"","signature":"x"},{"type":"text","text":"Hi"},{"type":"tool_use","id":"toolu_3","name":"Bash","input":{"command":"ls"}}]}}`,
			kinds: []Kind{KindThinking, KindText, KindToolUse},
			check: func(t *testing.T, it []Item) {
				for _, i := range it {
					if i.MessageID != "msg_1" {
						t.Fatalf("message id %q", i.MessageID)
					}
				}
				if it[0].Meta["durationMs"] != 1200.0 || it[2].Tool.Name != "Bash" || string(it[2].Tool.Input) != `{"command":"ls"}` {
					t.Fatalf("%+v %+v", it[0], it[2].Tool)
				}
			},
		},
		{
			name:  "unknown assistant block keeps its siblings",
			line:  `{"type":"assistant","uuid":"a2","message":{"id":"msg_2","content":[{"type":"quantum","v":1},{"type":"text","text":"after"}]}}`,
			kinds: []Kind{KindRaw, KindText},
		},
		{
			name:  "turn duration",
			line:  `{"type":"system","uuid":"s1","subtype":"turn_duration","durationMs":3200}`,
			kinds: []Kind{KindSystem},
			check: func(t *testing.T, it []Item) {
				if it[0].Label != "turn_duration" || it[0].Meta["durationMs"] != 3200.0 {
					t.Fatalf("%+v", it[0])
				}
			},
		},
		{
			name:  "compact boundary",
			line:  `{"type":"system","uuid":"s2","subtype":"compact_boundary","content":"Conversation compacted"}`,
			kinds: []Kind{KindCompact},
		},
		{
			name:  "queued human prompt",
			line:  `{"type":"attachment","uuid":"t1","attachment":{"type":"queued_command","prompt":[{"type":"text","text":"also do X"}],"origin":{"kind":"human"}}}`,
			kinds: []Kind{KindUser},
			check: func(t *testing.T, it []Item) {
				if it[0].Text != "also do X" || it[0].Meta["queued"] != true {
					t.Fatalf("%+v", it[0])
				}
			},
		},
		{
			name:  "queued task notification",
			line:  `{"type":"attachment","uuid":"t2","attachment":{"type":"queued_command","prompt":"<task-notification>x</task-notification>","origin":{"kind":"task-notification"}}}`,
			kinds: []Kind{KindNotice},
			check: labelIs("task-notification"),
		},
		{
			name:  "other attachment",
			line:  `{"type":"attachment","uuid":"t3","attachment":{"type":"environment","cwd":"/x"}}`,
			kinds: []Kind{KindAttachment},
			check: labelIs("environment"),
		},
		{
			name:  "pr link meta",
			line:  `{"type":"pr-link","sessionId":"s","prNumber":12,"prUrl":"https://example.test/pr/12","prRepository":"o/r"}`,
			kinds: []Kind{KindMeta},
			check: func(t *testing.T, it []Item) {
				if it[0].Label != "pr-link" || it[0].Meta["prNumber"] != 12.0 {
					t.Fatalf("%+v", it[0])
				}
			},
		},
		{
			name:  "reminder inside a prompt is split out",
			line:  `{"type":"user","uuid":"r1","message":{"content":"Please check this\n<system-reminder>internal note</system-reminder>"}}`,
			kinds: []Kind{KindUser, KindNotice},
			check: func(t *testing.T, it []Item) {
				if it[0].Text != "Please check this" || it[1].Text != "internal note" {
					t.Fatalf("%q %q", it[0].Text, it[1].Text)
				}
			},
		},
		{
			name:  "shell escape input",
			line:  `{"type":"user","uuid":"b1","message":{"content":"<bash-input>git status</bash-input>"}}`,
			kinds: []Kind{KindNotice},
			check: labelIs("bash-input"),
		},
		{
			name:  "no-content placeholder is dropped",
			line:  `{"type":"assistant","uuid":"nc","message":{"id":"m9","content":[{"type":"text","text":"(no content)"}]}}`,
			kinds: nil,
		},
		{"skipped bookkeeping", `{"type":"api-request-blob","hash":"h","message":{}}`, nil, nil},
		{"malformed json", `{"type":"user",`, []Kind{KindRaw}, nil},
		{"json array", `[1,2,3]`, []Kind{KindRaw}, nil},
		{"no type", `{"uuid":"x","message":{}}`, []Kind{KindRaw}, nil},
		{"unknown type", `{"type":"hologram","uuid":"h1"}`, []Kind{KindRaw}, nil},
		{"wrong-typed content", `{"type":"user","uuid":"w1","message":{"content":12345}}`, []Kind{KindRaw}, nil},
		{"wrong-typed message", `{"type":"assistant","uuid":"w2","message":"text"}`, []Kind{KindRaw}, nil},
		{"wrong-typed field", `{"type":"user","uuid":7}`, []Kind{KindRaw}, nil},
		{"blank line", `   `, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := parseOne(t, tt.line)
			var kinds []Kind
			for _, it := range items {
				kinds = append(kinds, it.Kind)
			}
			if strings.Join(kindStrings(kinds), ",") != strings.Join(kindStrings(tt.kinds), ",") {
				t.Fatalf("kinds = %v, want %v (%+v)", kinds, tt.kinds, items)
			}
			if tt.check != nil {
				tt.check(t, items)
			}
		})
	}
}

func labelIs(want string) func(t *testing.T, it []Item) {
	return func(t *testing.T, it []Item) {
		t.Helper()
		if it[0].Label != want {
			t.Fatalf("label = %q, want %q", it[0].Label, want)
		}
	}
}

func kindStrings(ks []Kind) []string {
	out := make([]string, len(ks))
	for i, k := range ks {
		out[i] = string(k)
	}
	return out
}

func TestDuplicatesAndSequence(t *testing.T) {
	p := NewParser()
	line := []byte(`{"type":"user","uuid":"same","message":{"content":"hi"}}`)
	first := p.Line(line)
	second := p.Line(line)
	third := p.Line([]byte(`{"type":"assistant","uuid":"next","message":{"id":"m","content":[{"type":"text","text":"a"},{"type":"text","text":"b"}]}}`))
	if len(first) != 1 || len(second) != 0 || p.Stats().Duplicates != 1 {
		t.Fatalf("duplicate handling: %v %v %+v", first, second, p.Stats())
	}
	if first[0].Seq != 0 || third[0].Seq != 1 || third[1].Seq != 2 {
		t.Fatalf("sequence: %d %d %d", first[0].Seq, third[0].Seq, third[1].Seq)
	}
}

func TestVersionStutterIsDeduped(t *testing.T) {
	p := NewParser()
	a := p.Line([]byte(`{"type":"assistant","uuid":"x1","message":{"id":"msg_s","content":[{"type":"text","text":"hello"}]}}`))
	b := p.Line([]byte(`{"type":"assistant","uuid":"x2","message":{"id":"msg_s","content":[{"type":"text","text":"hello"}]}}`))
	c := p.Line([]byte(`{"type":"assistant","uuid":"x3","message":{"id":"msg_s","content":[{"type":"tool_use","id":"t","name":"Bash","input":{}}]}}`))
	if len(a) != 1 || len(b) != 0 || len(c) != 1 {
		t.Fatalf("got %d %d %d items, want 1 0 1", len(a), len(b), len(c))
	}
}

func TestLargeOutputIsCapped(t *testing.T) {
	big := strings.Repeat("x", maxContent+100)
	line, _ := json.Marshal(map[string]any{
		"type": "user", "uuid": "big",
		"message":       map[string]any{"content": []any{map[string]any{"type": "tool_result", "tool_use_id": "t", "content": big}}},
		"toolUseResult": map[string]any{"stdout": big},
	})
	it := parseOne(t, string(line))
	r := it[0].Result
	if !r.Truncated || len(r.Content) != maxContent || len(r.Detail) > maxContent+200 {
		t.Fatalf("truncated=%v content=%d detail=%d", r.Truncated, len(r.Content), len(r.Detail))
	}
}

func TestStripANSI(t *testing.T) {
	tests := []struct{ in, want string }{
		{"plain", "plain"},
		{"\x1b[31mred\x1b[0m", "red"},
		{"\x1b]8;;https://x\x07link\x1b]8;;\x07", "link"},
		{"\x1b[?25lhidden cursor\x1b[?25h", "hidden cursor"},
	}
	for _, tt := range tests {
		if got := StripANSI(tt.in); got != tt.want {
			t.Errorf("StripANSI(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestSamples parses every anonymized real transcript. Real samples must
// produce no raw items: a raw item there means a record shape the parser
// should learn. The hand-written edge cases must parse without panicking.
func TestSamples(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "testdata", "transcripts", "*.jsonl"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no samples: %v", err)
	}
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			p := NewParser()
			var items []Item
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			sc := bufio.NewScanner(f)
			sc.Buffer(make([]byte, 1<<20), 64<<20)
			for sc.Scan() {
				items = append(items, p.Line(sc.Bytes())...)
			}
			if err := sc.Err(); err != nil {
				t.Fatal(err)
			}
			st := p.Stats()
			counts := map[Kind]int{}
			for _, it := range items {
				counts[it.Kind]++
			}
			t.Logf("lines=%d items=%d raw=%d dup=%d skipped=%v kinds=%v", st.Lines, st.Items, st.Raw, st.Duplicates, st.Skipped, counts)
			if filepath.Base(path) == "edge-cases.jsonl" {
				if st.Raw < 6 || counts[KindUser] != 1 || st.Duplicates != 1 {
					t.Fatalf("edge cases: raw=%d users=%d dup=%d", st.Raw, counts[KindUser], st.Duplicates)
				}
				return
			}
			for _, it := range items {
				if it.Kind == KindRaw {
					t.Errorf("raw item: %s: %.200s", it.Label, it.Raw)
				}
			}
			if counts[KindUser] == 0 || counts[KindText] == 0 {
				t.Fatalf("no conversation parsed: %v", counts)
			}
		})
	}
}
