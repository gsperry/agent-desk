package demo

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gsperry/agent-desk/internal/am"
	"github.com/gsperry/agent-desk/internal/transcript"
)

// Write empties its directory, so it refuses one holding anything but an
// earlier demo.
func TestWriteLeavesOtherDirectoriesAlone(t *testing.T) {
	dir := t.TempDir()
	keep := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(keep, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Write(dir, time.Now()); err == nil {
		t.Fatal("wrote over a directory with other files")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatal("a file in the directory was removed")
	}
	demo := filepath.Join(t.TempDir(), "demo")
	for i := 0; i < 2; i++ {
		if err := Write(demo, time.Now()); err != nil {
			t.Fatalf("write %d over an earlier demo: %v", i, err)
		}
	}
}

func TestWrite(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "demo")
	if err := Write(dir, time.Now()); err != nil {
		t.Fatal(err)
	}
	snap, err := am.Fixture{Path: filepath.Join(dir, "snapshot.json")}.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Sessions) != 7 || snap.Sessions[1].ID != Waiting || snap.Sessions[1].Status != "waiting" {
		t.Fatalf("sessions = %+v", snap.Sessions)
	}
	// Every record is one the parser knows: none ends up as a raw item.
	files, _ := filepath.Glob(filepath.Join(dir, "claude", "projects", "*", "*.jsonl"))
	if len(files) != 6+14 {
		t.Fatalf("transcripts = %d", len(files))
	}
	for _, f := range files {
		p := transcript.NewParser()
		fh, err := os.Open(f)
		if err != nil {
			t.Fatal(err)
		}
		sc := bufio.NewScanner(fh)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			for _, it := range p.Line(sc.Bytes()) {
				if it.Kind == transcript.KindRaw {
					t.Errorf("%s: raw item %q", filepath.Base(f), it.Label)
				}
			}
		}
		fh.Close()
	}
	var stats struct {
		Supported bool `json:"supported"`
	}
	data, _ := os.ReadFile(filepath.Join(dir, "stats.json"))
	if json.Unmarshal(data, &stats) != nil || !stats.Supported {
		t.Errorf("stats.json = %s", data)
	}
	if _, err := exec.LookPath("git"); err == nil {
		out, err := exec.Command("git", "-C", filepath.Join(dir, "repo"), "status", "--porcelain").Output()
		if err != nil || !strings.Contains(string(out), "useTotals.ts") {
			t.Errorf("repo status = %q, %v", out, err)
		}
	}
	// Writing again starts over.
	if err := os.WriteFile(filepath.Join(dir, "keys.log"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Write(dir, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "keys.log")); !os.IsNotExist(err) {
		t.Error("keys.log survived a rewrite")
	}
}

func startLive(t *testing.T) (string, am.Fixture) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "demo")
	if err := Write(dir, time.Now()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	l := &Live{Dir: dir, Every: 10 * time.Millisecond, Pause: 0.01}
	go func() {
		l.Run(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	time.Sleep(50 * time.Millisecond)
	return dir, am.Fixture{Path: filepath.Join(dir, "snapshot.json")}
}

func status(t *testing.T, f am.Fixture, id string) string {
	snap, err := f.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range snap.Sessions {
		if s.ID == id {
			return s.Status
		}
	}
	return ""
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func contains(t *testing.T, dir, conv, text string) bool {
	m, _ := filepath.Glob(filepath.Join(dir, "claude", "projects", "*", conv+".jsonl"))
	if len(m) == 0 {
		t.Fatalf("no transcript for %s", conv)
	}
	data, _ := os.ReadFile(m[0])
	return strings.Contains(string(data), text)
}

func TestLiveApprove(t *testing.T) {
	dir, f := startLive(t)
	if err := os.WriteFile(filepath.Join(dir, "keys.log"), []byte(Waiting+" Enter\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the agent's turn", func() bool {
		return status(t, f, Waiting) == "finished" && contains(t, dir, ConvID(2), "passed 20 runs in a row")
	})
	pane, _ := os.ReadFile(filepath.Join(dir, "panes", Waiting+".txt"))
	if strings.Contains(string(pane), "Do you want to make this edit") {
		t.Error("the prompt is still on screen")
	}
}

func TestLiveDecline(t *testing.T) {
	dir, f := startLive(t)
	if err := os.WriteFile(filepath.Join(dir, "keys.log"), []byte(Waiting+" Down Down Enter\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the declined reply", func() bool {
		return status(t, f, Waiting) == "finished" && contains(t, dir, ConvID(2), "left the fixture alone")
	})
	if contains(t, dir, ConvID(2), "passed 20 runs") {
		t.Error("a declined edit ran the tests")
	}
}

func TestLiveReply(t *testing.T) {
	dir, f := startLive(t)
	if err := f.Type(context.Background(), "d0000003", "Exempt internal keys, please."); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the demo's reply", func() bool {
		return status(t, f, "d0000003") == "finished" && contains(t, dir, ConvID(3), "This is a demo session")
	})
	// Earlier prompts are history: only the new one is answered.
	m, _ := filepath.Glob(filepath.Join(dir, "claude", "projects", "*", ConvID(3)+".jsonl"))
	data, _ := os.ReadFile(m[0])
	if n := strings.Count(string(data), "This is a demo session"); n != 1 {
		t.Errorf("replies = %d", n)
	}
}
