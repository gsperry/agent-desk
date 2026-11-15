package demo

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gsperry/agent-desk/internal/am"
)

// Live makes the demo answer back. Answering the waiting session's prompt
// plays out the agent's next steps, as in the site's video; a reply sent
// to any session gets a short note that this is a demo. It watches the
// files fixture mode writes: keys.log for answers, and transcripts for
// prompts typed into them.
type Live struct {
	Dir string
	// Every is how often it looks; 300 ms when zero.
	Every time.Duration
	// Pause scales the waits between an agent's steps; 1 when zero.
	Pause float64

	fixture  am.Fixture
	answered map[string]bool
	played   bool
}

func (l *Live) Run(ctx context.Context) {
	l.fixture = am.Fixture{Path: filepath.Join(l.Dir, "snapshot.json")}
	l.answered = map[string]bool{}
	// Prompts already in the transcripts are history, not replies to answer.
	for _, p := range l.typedPrompts() {
		l.answered[p.uuid] = true
	}
	every := l.Every
	if every == 0 {
		every = 300 * time.Millisecond
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		l.watchKeys(ctx)
		for _, p := range l.typedPrompts() {
			if !l.answered[p.uuid] {
				l.answered[p.uuid] = true
				go l.replyToPrompt(ctx, p)
			}
		}
	}
}

func (l *Live) wait(ctx context.Context, d time.Duration) bool {
	if l.Pause > 0 {
		d = time.Duration(float64(d) * l.Pause)
	}
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

// watchKeys plays the waiting session's answer once, when keys.log shows
// an Enter or Escape for it. Up and Down before the Enter pick the option,
// as they move Claude Code's selection.
func (l *Live) watchKeys(ctx context.Context) {
	if l.played {
		return
	}
	data, err := os.ReadFile(filepath.Join(l.Dir, "keys.log"))
	if err != nil {
		return
	}
	moves := 0
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) == 0 || f[0] != Waiting {
			continue
		}
		for _, k := range f[1:] {
			switch k {
			case "Down":
				moves++
			case "Up":
				moves--
			case "Escape":
				l.played = true
				go l.cancelled(ctx)
				return
			case "Enter":
				l.played = true
				go l.answer(ctx, moves)
				return
			}
		}
	}
}

// answer plays the waiting agent's turn after option choice: 0 and 1
// approve the edit, 2 declines it.
func (l *Live) answer(ctx context.Context, choice int) {
	path := l.transcript(ConvID(2))
	if path == "" {
		return
	}
	l.idlePane()
	_ = l.fixture.SetStatus(Waiting, "working")
	app := appender{path: path, seq: 100}
	if choice >= 2 {
		if !l.wait(ctx, 1200*time.Millisecond) {
			return
		}
		app.assistant([]record{{"type": "text", "text": "Understood, I left the fixture alone. The other option is to make the assertion wait for the session to be created instead of racing it. Want me to try that?"}}, 220)
		_ = l.fixture.SetStatus(Waiting, "finished")
		return
	}
	steps := []struct {
		pause time.Duration
		do    func()
	}{
		{600 * time.Millisecond, func() {
			app.assistant([]record{{"type": "tool_use", "id": "toolu_demo1", "name": "Edit", "input": record{"file_path": "/home/dev/src/platform/auth/testdata/fixture.go", "old_string": "TTL: 300 * time.Millisecond,", "new_string": "TTL: 5 * time.Second,"}}}, 180)
		}},
		{1600 * time.Millisecond, func() {
			app.result("toolu_demo1", "The file auth/testdata/fixture.go has been updated.")
			app.assistant([]record{{"type": "tool_use", "id": "toolu_demo2", "name": "Bash", "input": record{"command": "go test ./auth -run TestLogin -count=20", "description": "Run the login test 20 times"}}}, 260)
		}},
		{3200 * time.Millisecond, func() {
			app.result("toolu_demo2", "ok  \texample.com/platform/auth\t6.214s")
			app.assistant([]record{{"type": "text", "text": "Raised the session TTL in the test fixture from 300 ms to 5 s. The login test passed 20 runs in a row; production code is unchanged."}}, 410)
			app.write(record{"type": "system", "subtype": "turn_duration", "durationMs": 9000})
			_ = l.fixture.SetStatus(Waiting, "finished")
		}},
	}
	for _, s := range steps {
		if !l.wait(ctx, s.pause) {
			return
		}
		s.do()
	}
}

func (l *Live) cancelled(ctx context.Context) {
	l.idlePane()
	if l.wait(ctx, 400*time.Millisecond) {
		_ = l.fixture.SetStatus(Waiting, "idle")
	}
}

// replyNote is the demo's answer to anything typed to a session.
const replyNote = "This is a demo session, so nothing runs here. In a real session, Claude Code takes your reply from here, and Agent Desk shows its work as it goes."

func (l *Live) replyToPrompt(ctx context.Context, p typedPrompt) {
	if !l.wait(ctx, 800*time.Millisecond) {
		return
	}
	_ = l.fixture.SetStatus(p.session, "working")
	if !l.wait(ctx, 1800*time.Millisecond) {
		return
	}
	app := appender{path: p.path, seq: 500 + int(time.Now().UnixNano()%1000)}
	app.assistant([]record{{"type": "text", "text": replyNote}}, 60)
	_ = l.fixture.SetStatus(p.session, "finished")
}

// idlePane replaces the waiting session's prompt with Claude Code's empty
// prompt, as the screen looks once a dialog is answered.
func (l *Live) idlePane() {
	rule := strings.Repeat("─", 100)
	_ = os.WriteFile(filepath.Join(l.Dir, "panes", Waiting+".txt"), []byte(rule+"\n❯ \n"+rule+"\n  ⏵⏵ accept edits on\n"), 0o644)
}

func (l *Live) transcript(convID string) string {
	m, _ := filepath.Glob(filepath.Join(l.Dir, "claude", "projects", "*", convID+".jsonl"))
	if len(m) == 0 {
		return ""
	}
	return m[0]
}

type typedPrompt struct {
	uuid, path, session string
}

// typedPrompts finds prompts typed into demo sessions: fixture mode
// records them with a "fixture-" uuid.
func (l *Live) typedPrompts() []typedPrompt {
	snap, err := l.fixture.Snapshot(context.Background())
	if err != nil {
		return nil
	}
	var out []typedPrompt
	for _, s := range snap.Sessions {
		if s.AgentSessionID == "" {
			continue
		}
		path := l.transcript(s.AgentSessionID)
		if path == "" {
			continue
		}
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			line := sc.Bytes()
			if !strings.Contains(string(line), `"fixture-`) {
				continue
			}
			var r struct {
				UUID string `json:"uuid"`
			}
			if json.Unmarshal(line, &r) == nil && strings.HasPrefix(r.UUID, "fixture-") {
				out = append(out, typedPrompt{uuid: r.UUID, path: path, session: s.ID})
			}
		}
		f.Close()
	}
	return out
}

// appender adds records to a transcript as an agent would.
type appender struct {
	path string
	seq  int
}

func (a *appender) write(r record) {
	a.seq++
	r["uuid"] = "demo-" + time.Now().Format("150405") + "-" + itoa(a.seq)
	r["timestamp"] = ts(time.Now())
	data, err := json.Marshal(r)
	if err != nil {
		return
	}
	f, err := os.OpenFile(a.path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(data, '\n'))
}

func (a *appender) assistant(content []record, out int) {
	a.write(record{"type": "assistant", "message": record{"id": "msg_demo_" + itoa(a.seq+1), "model": "claude-opus-4-1", "role": "assistant",
		"content": content, "usage": record{"input_tokens": 400, "cache_read_input_tokens": 52000, "output_tokens": out}}})
}

func (a *appender) result(id, content string) {
	a.write(record{"type": "user", "message": record{"role": "user", "content": []record{{"type": "tool_result", "tool_use_id": id, "content": content}}}})
}

func itoa(n int) string { return strconv.Itoa(n) }
