package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gsperry/agent-desk/internal/transcript"
)

func parse(t *testing.T, lines ...string) []transcript.Item {
	t.Helper()
	p := transcript.NewParser()
	var items []transcript.Item
	for _, l := range lines {
		items = append(items, p.Line([]byte(l))...)
	}
	return items
}

func call(id string, background bool) string {
	bg := "false"
	if background {
		bg = "true"
	}
	return `{"type":"assistant","uuid":"u-` + id + `","message":{"id":"m-` + id + `","content":[{"type":"tool_use","id":"` + id + `","name":"Agent","input":{"description":"d","run_in_background":` + bg + `}}]}}`
}

func result(id string, async, isErr bool) string {
	detail := `{}`
	if async {
		detail = `{"isAsync":true,"agentId":"x"}`
	}
	e := "false"
	if isErr {
		e = "true"
	}
	return `{"type":"user","uuid":"r-` + id + `","message":{"content":[{"type":"tool_result","tool_use_id":"` + id + `","is_error":` + e + `,"content":"ok"}]},"toolUseResult":` + detail + `}`
}

func notification(id, status string) string {
	return `{"type":"user","uuid":"n-` + id + `","origin":{"kind":"task-notification"},"message":{"content":"<task-notification>\n<task-id>a</task-id>\n<tool-use-id>` + id + `</tool-use-id>\n<status>` + status + `</status>\n<summary>s</summary>\n</task-notification>"}}`
}

func TestStatuses(t *testing.T) {
	items := parse(t,
		call("bg-done", true), result("bg-done", true, false), notification("bg-done", "completed"),
		call("bg-failed", true), result("bg-failed", true, false), notification("bg-failed", "failed"),
		call("bg-running", true), result("bg-running", true, false),
		call("fg-done", false), result("fg-done", false, false),
		call("fg-error", false), result("fg-error", false, true),
		call("bg-killed", true), result("bg-killed", true, false), notification("bg-killed", "killed"),
	)
	got := Statuses(items)
	want := map[string]string{"bg-done": StatusCompleted, "bg-failed": StatusFailed, "fg-done": StatusCompleted, "fg-error": StatusFailed, "bg-killed": StatusStopped}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	if _, ok := got["bg-running"]; ok {
		t.Errorf("bg-running has a status: %q", got["bg-running"])
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestListAndTranscript(t *testing.T) {
	conv := filepath.Join(t.TempDir(), "projects", "p", "conv-1.jsonl")
	write(t, conv, "")
	dir := Dir(conv)
	write(t, filepath.Join(dir, "agent-aaa.meta.json"), `{"agentType":"executor","description":"Build it","toolUseId":"bg-done","spawnDepth":1,"pickedEffortModel":"opus"}`)
	write(t, filepath.Join(dir, "agent-aaa.jsonl"), `{"type":"user","uuid":"s1","timestamp":"2026-11-10T10:00:00Z","isSidechain":true,"message":{"content":"Build it"}}`+"\n"+`{"type":"assistant","uuid":"s2","timestamp":"2026-11-10T10:01:00Z","message":{"id":"x","content":[{"type":"text","text":"Built."}]}}`+"\n")
	write(t, filepath.Join(dir, "agent-bbb.meta.json"), `{"agentType":"auditor","description":"Audit it","toolUseId":"bg-running","spawnDepth":1}`)
	write(t, filepath.Join(dir, "agent-bbb.jsonl"), `{"type":"user","uuid":"t1","timestamp":"2026-11-10T11:00:00Z","message":{"content":"Audit it"}}`+"\n")
	write(t, filepath.Join(dir, "agent-..evil.meta.json"), `{}`)
	write(t, filepath.Join(dir, "agent-ccc.meta.json"), `not json`)

	parent := parse(t, call("bg-done", true), result("bg-done", true, false), notification("bg-done", "completed"), call("bg-running", true), result("bg-running", true, false))
	agents, err := List(conv, parent)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, a := range agents {
		got = append(got, a.ID+"/"+a.Type+"/"+a.Status+"/"+a.Started)
	}
	want := "bbb/auditor/running/2026-11-10T11:00:00Z,aaa/executor/completed/2026-11-10T10:00:00Z"
	if strings.Join(got, ",") != want {
		t.Fatalf("agents = %v\nwant %s", got, want)
	}
	if agents[1].Model != "opus" || agents[1].Description != "Build it" || agents[1].LastActivity == "" {
		t.Fatalf("agent detail %+v", agents[1])
	}

	items, err := Transcript(conv, "aaa")
	if err != nil || len(items) != 2 || items[1].Text != "Built." {
		t.Fatalf("transcript %+v, %v", items, err)
	}
	for _, id := range []string{"zzz", "../../etc/passwd", ""} {
		if _, err := Transcript(conv, id); err != ErrNotFound {
			t.Errorf("Transcript(%q) err = %v", id, err)
		}
	}
	none, err := List(filepath.Join(t.TempDir(), "other.jsonl"), nil)
	if err != nil || len(none) != 0 {
		t.Fatalf("no subagents: %v %v", none, err)
	}
}
