// Package agents lists the subagents a Claude conversation spawned and
// reads their transcripts.
//
// Claude Code writes each subagent to <conversation>/subagents/
// agent-<id>.jsonl, with agent-<id>.meta.json naming the parent tool call.
// Background agents report completion to the parent as a task-notification
// carrying that tool call's id; foreground agents return their result
// directly.
package agents

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gsperry/agent-desk/internal/transcript"
)

const (
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusStopped   = "stopped"
)

type Agent struct {
	ID           string `json:"id"`
	ToolUseID    string `json:"toolUseId"`
	Type         string `json:"type"`
	Description  string `json:"description"`
	Depth        int    `json:"depth"`
	Model        string `json:"model,omitempty"`
	Status       string `json:"status"`
	Started      string `json:"started,omitempty"`
	LastActivity string `json:"lastActivity,omitempty"`
}

var ErrNotFound = errors.New("subagent not found")

var validID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Active counts the subagents whose transcripts changed within the last
// window: a cheap sign of work in progress that needs no parsing, for the
// overview. A subagent waiting on a long command may not count.
func Active(conversationPath string, window time.Duration, now time.Time) int {
	entries, err := os.ReadDir(Dir(conversationPath))
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "agent-") || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		if info, err := e.Info(); err == nil && now.Sub(info.ModTime()) <= window {
			n++
		}
	}
	return n
}

// Dir is where a conversation's subagent files live.
func Dir(conversationPath string) string {
	return filepath.Join(strings.TrimSuffix(conversationPath, ".jsonl"), "subagents")
}

type meta struct {
	AgentType         string `json:"agentType"`
	Description       string `json:"description"`
	ToolUseID         string `json:"toolUseId"`
	SpawnDepth        int    `json:"spawnDepth"`
	PickedEffortModel string `json:"pickedEffortModel"`
}

// List returns the conversation's subagents, newest first, with a status
// derived from the parent's items.
func List(conversationPath string, parent []transcript.Item) ([]Agent, error) {
	files, err := filepath.Glob(filepath.Join(Dir(conversationPath), "agent-*.meta.json"))
	if err != nil {
		return nil, err
	}
	metas := map[string]meta{}
	toolUse := map[string]string{}
	for _, f := range files {
		id := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(f), "agent-"), ".meta.json")
		if !validID.MatchString(id) {
			continue
		}
		var m meta
		data, err := os.ReadFile(f)
		if err != nil || json.Unmarshal(data, &m) != nil {
			continue
		}
		metas[f] = m
		toolUse[id] = m.ToolUseID
	}
	statuses := statusesOf(parent, toolUse)
	agents := make([]Agent, 0, len(metas))
	for _, f := range files {
		m, ok := metas[f]
		if !ok {
			continue
		}
		id := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(f), "agent-"), ".meta.json")
		a := Agent{ID: id, ToolUseID: m.ToolUseID, Type: m.AgentType, Description: m.Description, Depth: m.SpawnDepth, Model: m.PickedEffortModel}
		a.Status = statuses[m.ToolUseID]
		if a.Status == "" {
			a.Status = StatusRunning
		}
		jsonl := strings.TrimSuffix(f, ".meta.json") + ".jsonl"
		if info, err := os.Stat(jsonl); err == nil {
			a.LastActivity = info.ModTime().UTC().Format(time.RFC3339)
		}
		a.Started = firstTimestamp(jsonl)
		agents = append(agents, a)
	}
	sort.SliceStable(agents, func(i, j int) bool { return agents[i].Started > agents[j].Started })
	return agents, nil
}

var (
	toolUseTag = regexp.MustCompile(`<tool-use-id>\s*([^<\s]+)\s*</tool-use-id>`)
	statusTag  = regexp.MustCompile(`<status>\s*([^<\s]+)\s*</status>`)
	taskIDTag  = regexp.MustCompile(`<task-id>\s*([^<\s]+)\s*</task-id>`)
	// resumedNote is the notification Claude Code sends when the user wakes
	// a finished subagent with a message from its view.
	resumedNote = regexp.MustCompile(`(?i)was resumed|is running again`)
)

// Statuses maps a parent tool call id to its subagent's outcome: from
// task notifications for background agents, and from the tool result for
// foreground ones. A call with neither is still running.
func Statuses(parent []transcript.Item) map[string]string {
	return statusesOf(parent, nil)
}

// statusesOf is Statuses, also following resumptions, which name the
// subagent by its agent id; toolUse maps an agent id to the tool call that
// started it. A finished subagent runs again when the parent's SendMessage
// resumes it ("Resuming agent …") or the user wakes it from its view, until
// a later notification reports it again.
func statusesOf(parent []transcript.Item, toolUse map[string]string) map[string]string {
	out := map[string]string{}
	// at is when each subagent's status last changed. Claude Code can
	// write a copy of an earlier event after a later one (a "resumed"
	// notice after the "completed" that followed it), so an event older
	// than the status it would replace is skipped.
	at := map[string]time.Time{}
	set := func(call, status, when string) {
		t, err := time.Parse(time.RFC3339Nano, when)
		if err == nil {
			if last, ok := at[call]; ok && t.Before(last) {
				return
			}
			at[call] = t
		}
		out[call] = status
	}
	agentCalls := map[string]bool{}
	// sends maps a SendMessage call to the agent it addresses, and stops a
	// TaskStop call to the task it ends.
	sends := map[string]string{}
	stops := map[string]string{}
	for _, it := range parent {
		switch {
		case it.Kind == transcript.KindToolUse && it.Tool != nil && it.Tool.Name == "SendMessage":
			if to := inputField(it.Tool.Input, "to"); to != "" {
				sends[it.Tool.ID] = to
			}
		case it.Kind == transcript.KindToolUse && it.Tool != nil && it.Tool.Name == "TaskStop":
			if task := inputField(it.Tool.Input, "task_id"); task != "" {
				stops[it.Tool.ID] = task
			}
		case it.Kind == transcript.KindToolResult && it.Result != nil && sends[it.Result.ToolUseID] != "":
			call, ok := toolUse[sends[it.Result.ToolUseID]]
			if ok && !it.Result.IsError && (strings.Contains(it.Result.Content, "resumedAgentId") || strings.Contains(it.Result.Content, "Resuming agent")) {
				set(call, StatusRunning, it.Time)
			}
		case it.Kind == transcript.KindToolResult && it.Result != nil && stops[it.Result.ToolUseID] != "":
			// The parent's TaskStop on a subagent ("Successfully stopped
			// task: …"); a task id that is a shell names no subagent.
			if call, ok := toolUse[stops[it.Result.ToolUseID]]; ok && !it.Result.IsError && strings.Contains(it.Result.Content, "stopped") {
				set(call, StatusStopped, it.Time)
			}
		case it.Kind == transcript.KindToolUse && it.Tool != nil && (it.Tool.Name == "Agent" || it.Tool.Name == "Task"):
			agentCalls[it.Tool.ID] = true
		case it.Kind == transcript.KindToolResult && it.Result != nil && agentCalls[it.Result.ToolUseID]:
			if isAsync(it.Result.Detail) {
				continue
			}
			if it.Result.IsError {
				set(it.Result.ToolUseID, StatusFailed, it.Time)
			} else {
				set(it.Result.ToolUseID, StatusCompleted, it.Time)
			}
		case it.Kind == transcript.KindNotice && it.Label == "task-notification":
			// The task id is the subagent's agent id, the same in every
			// notification. A resumed subagent's notifications carry another
			// tool call id than the one that started it, so the task id
			// decides when it names a known subagent.
			st := statusTag.FindStringSubmatch(it.Text)
			call := ""
			if task := taskIDTag.FindStringSubmatch(it.Text); task != nil {
				call = toolUse[task[1]]
			}
			if call == "" {
				if id := toolUseTag.FindStringSubmatch(it.Text); id != nil {
					call = id[1]
				}
			}
			switch {
			case call == "":
			case st != nil:
				set(call, normalize(st[1]), it.Time)
			case resumedNote.MatchString(it.Text):
				set(call, StatusRunning, it.Time)
			}
		}
	}
	return out
}

// inputField is one string field of a tool call's input, such as the
// agent a SendMessage addresses ("to") or the task a TaskStop ends
// ("task_id").
func inputField(input any, name string) string {
	var in map[string]any
	data, err := json.Marshal(input)
	if err != nil || json.Unmarshal(data, &in) != nil {
		return ""
	}
	v, _ := in[name].(string)
	return v
}

func normalize(s string) string {
	switch strings.ToLower(s) {
	case "completed", "complete", "done", "success":
		return StatusCompleted
	case "failed", "error", "errored":
		return StatusFailed
	case "killed", "stopped", "cancelled", "canceled":
		return StatusStopped
	}
	return strings.ToLower(s)
}

func isAsync(detail json.RawMessage) bool {
	var d struct {
		IsAsync bool `json:"isAsync"`
	}
	return json.Unmarshal(detail, &d) == nil && d.IsAsync
}

func firstTimestamp(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	for i := 0; i < 20 && sc.Scan(); i++ {
		var r struct {
			Timestamp string `json:"timestamp"`
		}
		if json.Unmarshal(sc.Bytes(), &r) == nil && r.Timestamp != "" {
			return r.Timestamp
		}
	}
	return ""
}

// maxItems caps a subagent transcript sent to the browser.
const maxItems = 3000

// Transcript parses one subagent's transcript.
func Transcript(conversationPath, id string) ([]transcript.Item, error) {
	if !validID.MatchString(id) {
		return nil, ErrNotFound
	}
	f, err := os.Open(filepath.Join(Dir(conversationPath), "agent-"+id+".jsonl"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var modTime time.Time
	if info, err := f.Stat(); err == nil {
		modTime = info.ModTime()
	}
	p := transcript.NewParser()
	var items []transcript.Item
	r := bufio.NewReaderSize(f, 1<<20)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 && !transcript.Unfinished(line, modTime, time.Now()) {
			items = append(items, p.Line(line)...)
		}
		if err != nil {
			break
		}
	}
	if len(items) > maxItems {
		items = items[len(items)-maxItems:]
	}
	if items == nil {
		items = []transcript.Item{}
	}
	return items, nil
}
