// Package summary writes catch-up summaries of a session with the user's
// own Claude Code CLI (`claude -p`). It is the one place Agent Desk runs a
// model, and only when the user has turned summaries on and asks for one:
// each summary spends tokens on the user's provider.
//
// The CLI runs with no tools, no MCP servers, no session persistence and a
// one-line system prompt, in an empty temporary directory, with the
// transcript excerpt on stdin.
package summary

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/gsperry/agent-desk/internal/export"
	"github.com/gsperry/agent-desk/internal/hub"
	"github.com/gsperry/agent-desk/internal/send"
	"github.com/gsperry/agent-desk/internal/transcript"
)

// Runner runs a command in a directory with input on stdin.
type Runner interface {
	RunInput(ctx context.Context, dir string, input []byte, name string, args ...string) ([]byte, error)
}

// Transcripts gives the newest items of a session's transcript.
type Transcripts interface {
	Window(ctx context.Context, sessionID string, before, limit int) hub.Page
}

type Summarizer struct {
	Runner      Runner
	Transcripts Transcripts
	// Bin is the Claude Code CLI, "claude" unless set.
	Bin string

	mu    sync.Mutex
	cache map[string]Result
}

// Result is a summary and what it cost.
type Result struct {
	Text         string  `json:"text"`
	Model        string  `json:"model"`
	InputTokens  int64   `json:"inputTokens"`
	OutputTokens int64   `json:"outputTokens"`
	CostUSD      float64 `json:"costUSD"`
	Items        int     `json:"items"`
	Cached       bool    `json:"cached,omitempty"`
}

const (
	// excerptChars caps the transcript excerpt; the newest part is kept.
	excerptChars = 40000
	windowItems  = 2000
	maxSummary   = 4000
)

var modelName = regexp.MustCompile(`^[A-Za-z0-9._:\[\]-]{1,100}$`)

const reasonInvalid send.Reason = "invalid"

const systemPrompt = "You write short catch-up notes about coding agent sessions."

const instructions = `You are writing a catch-up note for a developer who stepped away from a coding agent's session. From the transcript excerpt below, write at most six short bullet points: what the agent did, what it is doing now, anything that failed or looks risky, and what it needs from the developer. Plain text, no heading, no preamble.

Transcript excerpt:

`

// Summarize writes a summary of the session's items after since (a Seq;
// -1 for the newest window). An unchanged request is answered from cache.
func (s *Summarizer) Summarize(ctx context.Context, sessionID string, since int, model string) (Result, error) {
	model = strings.TrimSpace(model)
	if model == "" || !modelName.MatchString(model) {
		return Result{}, &send.Refused{Reason: reasonInvalid, Message: "Choose a model name such as haiku or sonnet in Settings."}
	}
	page := s.Transcripts.Window(ctx, sessionID, -1, windowItems)
	if page.Status != hub.DocOK {
		return Result{}, &send.Refused{Reason: send.Reason(page.Status), Message: "This session has no transcript to summarize."}
	}
	var items []transcript.Item
	for _, it := range page.Items {
		if it.Seq > since {
			items = append(items, it)
		}
	}
	excerpt := export.Markdown(export.Header{Title: "Session " + sessionID}, items)
	if len(items) == 0 || strings.Count(excerpt, "\n## ") == 0 {
		return Result{}, &send.Refused{Reason: "nothing_new", Message: "Nothing new to summarize since you last looked."}
	}
	if len(excerpt) > excerptChars {
		excerpt = "…\n" + excerpt[len(excerpt)-excerptChars:]
	}
	last := items[len(items)-1].Seq
	key := fmt.Sprintf("%s|%d|%d|%s", sessionID, since, last, model)
	s.mu.Lock()
	if r, ok := s.cache[key]; ok {
		s.mu.Unlock()
		r.Cached = true
		return r, nil
	}
	s.mu.Unlock()

	dir, err := os.MkdirTemp("", "agent-desk-summary-")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(dir)
	bin := s.Bin
	if bin == "" {
		bin = "claude"
	}
	out, err := s.Runner.RunInput(ctx, dir, []byte(instructions+excerpt), bin, Args(model)...)
	if err != nil {
		return Result{}, fmt.Errorf("claude -p: %w", err)
	}
	r, err := parse(out)
	if err != nil {
		return Result{}, err
	}
	r.Model, r.Items = model, len(items)
	s.mu.Lock()
	if s.cache == nil {
		s.cache = map[string]Result{}
	}
	s.cache[key] = r
	s.mu.Unlock()
	return r, nil
}

// Args is the argv after the binary. Values use --flag=value so none can
// be read as another flag.
func Args(model string) []string {
	return []string{
		"-p",
		"--output-format=json",
		"--model=" + model,
		"--no-session-persistence",
		"--tools=",
		"--strict-mcp-config",
		// A one-line system prompt in place of Claude Code's default, which
		// is several thousand tokens a summary does not need.
		"--system-prompt=" + systemPrompt,
	}
}

// result is the part of `claude -p --output-format json` a summary reads.
type result struct {
	Result       string   `json:"result"`
	IsError      bool     `json:"is_error"`
	TotalCostUSD *float64 `json:"total_cost_usd"`
	Usage        struct {
		InputTokens         int64 `json:"input_tokens"`
		OutputTokens        int64 `json:"output_tokens"`
		CacheReadTokens     int64 `json:"cache_read_input_tokens"`
		CacheCreationTokens int64 `json:"cache_creation_input_tokens"`
	} `json:"usage"`
}

func parse(out []byte) (Result, error) {
	var r result
	if err := json.Unmarshal(out, &r); err != nil {
		return Result{}, errors.New("claude -p did not return JSON")
	}
	text := strings.TrimSpace(transcript.StripANSI(r.Result))
	if r.IsError || text == "" {
		if text == "" {
			text = "no result"
		}
		return Result{}, fmt.Errorf("claude -p failed: %.300s", text)
	}
	if len(text) > maxSummary {
		text = text[:maxSummary] + "…"
	}
	res := Result{
		Text:         text,
		InputTokens:  r.Usage.InputTokens + r.Usage.CacheReadTokens + r.Usage.CacheCreationTokens,
		OutputTokens: r.Usage.OutputTokens,
	}
	if r.TotalCostUSD != nil {
		res.CostUSD = *r.TotalCostUSD
	}
	return res, nil
}
