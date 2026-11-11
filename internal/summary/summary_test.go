package summary

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/gsperry/agent-desk/internal/hub"
	"github.com/gsperry/agent-desk/internal/send"
	"github.com/gsperry/agent-desk/internal/transcript"
)

type fakeRunner struct {
	calls int
	dir   string
	input string
	name  string
	args  []string
	out   string
	err   error
}

func (f *fakeRunner) RunInput(_ context.Context, dir string, input []byte, name string, args ...string) ([]byte, error) {
	f.calls++
	f.dir, f.input, f.name, f.args = dir, string(input), name, args
	if _, err := os.Stat(dir); err != nil {
		return nil, errors.New("working directory missing")
	}
	return []byte(f.out), f.err
}

type fakeTranscripts struct{ page hub.Page }

func (f fakeTranscripts) Window(context.Context, string, int, int) hub.Page { return f.page }

const okJSON = `{"type":"result","subtype":"success","is_error":false,"result":"- Fixed the flaky test.\n- Needs your OK to merge.","total_cost_usd":0.0021,"usage":{"input_tokens":900,"cache_read_input_tokens":100,"cache_creation_input_tokens":0,"output_tokens":40}}`

func page() hub.Page {
	return hub.Page{Status: hub.DocOK, Items: []transcript.Item{
		{Seq: 1, Kind: transcript.KindUser, Text: "old prompt"},
		{Seq: 2, Kind: transcript.KindText, Text: "old reply"},
		{Seq: 3, Kind: transcript.KindUser, Text: "Fix the flaky login test"},
		{Seq: 4, Kind: transcript.KindText, Text: "Fixed it; the TTL was too short."},
	}}
}

func TestSummarize(t *testing.T) {
	r := &fakeRunner{out: okJSON}
	s := &Summarizer{Runner: r, Transcripts: fakeTranscripts{page()}}
	got, err := s.Summarize(context.Background(), "s1", 2, "haiku")
	if err != nil {
		t.Fatal(err)
	}
	want := Result{Text: "- Fixed the flaky test.\n- Needs your OK to merge.", Model: "haiku", InputTokens: 1000, OutputTokens: 40, CostUSD: 0.0021, Items: 2}
	if got != want {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
	if r.name != "claude" || strings.Join(r.args, " ") != "-p --output-format=json --model=haiku --no-session-persistence --tools= --strict-mcp-config --system-prompt=You write short catch-up notes about coding agent sessions." {
		t.Fatalf("argv = %s %v", r.name, r.args)
	}
	if !strings.Contains(r.input, "Fix the flaky login test") || strings.Contains(r.input, "old prompt") {
		t.Fatalf("stdin should hold only items after since:\n%s", r.input)
	}
	if !strings.Contains(r.dir, "agent-desk-summary-") {
		t.Fatalf("dir = %q", r.dir)
	}
	if _, err := os.Stat(r.dir); !os.IsNotExist(err) {
		t.Fatal("temporary directory was not removed")
	}
	// The same request again is answered from cache without running claude.
	again, err := s.Summarize(context.Background(), "s1", 2, "haiku")
	if err != nil || !again.Cached || r.calls != 1 {
		t.Fatalf("cache: %+v %v calls=%d", again, err, r.calls)
	}
}

func TestSummarizeRefusals(t *testing.T) {
	tests := []struct {
		name   string
		page   hub.Page
		since  int
		model  string
		reason send.Reason
	}{
		{"bad model", page(), -1, "opus --dangerously", reasonInvalid},
		{"empty model", page(), -1, " ", reasonInvalid},
		{"nothing new", page(), 4, "haiku", "nothing_new"},
		{"no transcript", hub.Page{Status: hub.DocPending}, -1, "haiku", send.Reason(hub.DocPending)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &fakeRunner{out: okJSON}
			_, err := (&Summarizer{Runner: r, Transcripts: fakeTranscripts{tt.page}}).Summarize(context.Background(), "s1", tt.since, tt.model)
			var refused *send.Refused
			if !errors.As(err, &refused) || refused.Reason != tt.reason || r.calls != 0 {
				t.Fatalf("err = %v calls = %d, want refusal %q without running claude", err, r.calls, tt.reason)
			}
		})
	}
}

func TestSummarizeFailures(t *testing.T) {
	tests := []struct {
		name, out string
		err       error
		want      string
	}{
		{"not json", "Error: not logged in", nil, "did not return JSON"},
		{"error result", `{"is_error":true,"result":"model not found"}`, nil, "model not found"},
		{"empty result", `{"is_error":false,"result":"  "}`, nil, "no result"},
		{"process error", "", errors.New("exit 1"), "exit 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Summarizer{Runner: &fakeRunner{out: tt.out, err: tt.err}, Transcripts: fakeTranscripts{page()}}
			if _, err := s.Summarize(context.Background(), "s1", -1, "haiku"); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
		})
	}
}
