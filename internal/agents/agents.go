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
	statuses := Statuses(parent)
	agents := make([]Agent, 0, len(files))
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
)

// Statuses maps a parent tool call id to its subagent's outcome: from
// task notifications for background agents, and from the tool result for
// foreground ones. A call with neither is still running.
func Statuses(parent []transcript.Item) map[string]string {
	out := map[string]string{}
	agentCalls := map[string]bool{}
	for _, it := range parent {
		switch {
		case it.Kind == transcript.KindToolUse && it.Tool != nil && (it.Tool.Name == "Agent" || it.Tool.Name == "Task"):
			agentCalls[it.Tool.ID] = true
		case it.Kind == transcript.KindToolResult && it.Result != nil && agentCalls[it.Result.ToolUseID]:
			if isAsync(it.Result.Detail) {
				continue
			}
			if it.Result.IsError {
				out[it.Result.ToolUseID] = StatusFailed
			} else {
				out[it.Result.ToolUseID] = StatusCompleted
			}
		case it.Kind == transcript.KindNotice && it.Label == "task-notification":
			id := toolUseTag.FindStringSubmatch(it.Text)
			st := statusTag.FindStringSubmatch(it.Text)
			if id != nil && st != nil {
				out[id[1]] = normalize(st[1])
			}
		}
	}
	return out
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
	p := transcript.NewParser()
	var items []transcript.Item
	r := bufio.NewReaderSize(f, 1<<20)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
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
