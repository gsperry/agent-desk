// Package overview summarizes what each session is doing from the end of
// its transcript, for the fleet overview.
package overview

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gsperry/agent-desk/internal/hub"
	"github.com/gsperry/agent-desk/internal/locate"
	"github.com/gsperry/agent-desk/internal/transcript"
)

// Summary is one session's card.
type Summary struct {
	Activity      string  `json:"activity,omitempty"`
	ActivityKind  string  `json:"activityKind,omitempty"`
	LastReply     string  `json:"lastReply,omitempty"`
	LastPrompt    string  `json:"lastPrompt,omitempty"`
	PromptAt      string  `json:"promptAt,omitempty"`
	ContextTokens float64 `json:"contextTokens,omitempty"`
	Model         string  `json:"model,omitempty"`
	Updated       string  `json:"updated,omitempty"`
}

type Sessions interface {
	State() hub.State
}

type Overview struct {
	Sessions Sessions
	Locator  *locate.Locator

	mu    sync.Mutex
	cache map[string]cached
}

type cached struct {
	size    int64
	modTime time.Time
	summary Summary
}

// tailBytes is how much of the end of a transcript a summary reads.
const tailBytes = 512 << 10

// Summaries returns a summary for every unarchived Claude session with a
// transcript, keyed by session id.
func (o *Overview) Summaries(ctx context.Context) map[string]Summary {
	out := map[string]Summary{}
	for _, s := range o.Sessions.State().Sessions {
		if s.Archived || s.Tool != "claude" || s.AgentSessionID == "" {
			continue
		}
		path, err := o.Locator.Find(ctx, s.AgentSessionID, s.Environment)
		if err != nil {
			continue
		}
		if sum, err := o.summary(path); err == nil {
			out[s.ID] = sum
		}
	}
	return out
}

func (o *Overview) summary(path string) (Summary, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Summary{}, err
	}
	o.mu.Lock()
	c, ok := o.cache[path]
	o.mu.Unlock()
	if ok && c.size == info.Size() && c.modTime.Equal(info.ModTime()) {
		return c.summary, nil
	}
	sum, err := Summarize(path)
	if err != nil {
		return Summary{}, err
	}
	sum.Updated = info.ModTime().UTC().Format(time.RFC3339)
	o.mu.Lock()
	if o.cache == nil {
		o.cache = map[string]cached{}
	}
	o.cache[path] = cached{size: info.Size(), modTime: info.ModTime(), summary: sum}
	o.mu.Unlock()
	return sum, nil
}

// Summarize reads the end of a transcript and picks out the latest
// activity, reply, prompt and context size.
func Summarize(path string) (Summary, error) {
	f, err := os.Open(path)
	if err != nil {
		return Summary{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return Summary{}, err
	}
	start := max(info.Size()-tailBytes, 0)
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return Summary{}, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return Summary{}, err
	}
	lines := strings.Split(string(data), "\n")
	if start > 0 && len(lines) > 0 {
		// The first line is cut mid-record.
		lines = lines[1:]
	}
	p := transcript.NewParser()
	var items []transcript.Item
	for _, l := range lines {
		items = append(items, p.Line([]byte(l))...)
	}
	return fromItems(items), nil
}

func fromItems(items []transcript.Item) Summary {
	var s Summary
	for i := len(items) - 1; i >= 0; i-- {
		it := items[i]
		if s.Activity == "" {
			switch it.Kind {
			case transcript.KindToolUse:
				s.Activity, s.ActivityKind = toolActivity(it), "tool"
			case transcript.KindText:
				s.Activity, s.ActivityKind = firstLine(it.Text), "reply"
			case transcript.KindUser:
				s.Activity, s.ActivityKind = firstLine(it.Text), "prompt"
			}
		}
		if s.LastReply == "" && it.Kind == transcript.KindText {
			s.LastReply = clip(it.Text, 280)
		}
		if s.LastPrompt == "" && it.Kind == transcript.KindUser {
			s.LastPrompt, s.PromptAt = clip(it.Text, 200), it.Time
		}
		if s.ContextTokens == 0 {
			if u, ok := it.Meta["usage"].(map[string]any); ok {
				s.ContextTokens, _ = u["contextTokens"].(float64)
				s.Model, _ = u["model"].(string)
			}
		}
		if s.Activity != "" && s.LastReply != "" && s.LastPrompt != "" && s.ContextTokens != 0 {
			break
		}
	}
	return s
}

// toolActivity names a tool call by its most telling input.
func toolActivity(it transcript.Item) string {
	if it.Tool == nil {
		return ""
	}
	var in map[string]any
	_ = json.Unmarshal(it.Tool.Input, &in)
	for _, k := range []string{"description", "command", "file_path", "pattern", "url", "query", "prompt"} {
		if v, ok := in[k].(string); ok && strings.TrimSpace(v) != "" {
			return it.Tool.Name + ": " + firstLine(v)
		}
	}
	return it.Tool.Name
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return clip(s, 160)
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
