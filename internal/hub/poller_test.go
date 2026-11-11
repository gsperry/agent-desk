package hub

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gsperry/agent-desk/internal/am"
	"github.com/gsperry/agent-desk/internal/grouping"
)

type fakeClient struct {
	mu    sync.Mutex
	snap  am.Snapshot
	err   error
	calls atomic.Int32
}

func (f *fakeClient) Snapshot(context.Context) (am.Snapshot, error) {
	f.calls.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.snap, f.err
}

func (f *fakeClient) StartServe(context.Context) error { return nil }

func (f *fakeClient) ProfileVar(context.Context, string, string) (string, error) { return "", nil }

func (f *fakeClient) set(snap am.Snapshot, err error) {
	f.mu.Lock()
	f.snap, f.err = snap, err
	f.mu.Unlock()
}

type noGit struct{}

func (noGit) Run(context.Context, string, ...string) ([]byte, error) {
	return nil, errors.New("not a repo")
}

func newPoller(c *fakeClient) *Poller {
	return &Poller{Client: c, Resolver: &grouping.Resolver{Runner: noGit{}}}
}

func snapshot(statuses ...string) am.Snapshot {
	s := am.Snapshot{Version: 1, ManagerAwake: true}
	for i, st := range statuses {
		s.Sessions = append(s.Sessions, am.Session{ID: string(rune('a' + i)), Name: "s", Tool: "claude", Directory: "/tmp/p", Status: st})
	}
	return s
}

func TestPollPublishesOnlyChanges(t *testing.T) {
	c := &fakeClient{}
	p := newPoller(c)
	ch, cancel := p.Subscribe()
	defer cancel()

	steps := []struct {
		name        string
		snap        am.Snapshot
		err         error
		wantPublish bool
		check       func(t *testing.T, st State)
	}{
		{"first snapshot", snapshot("working"), nil, true, func(t *testing.T, st State) {
			if len(st.Sessions) != 1 || st.Sessions[0].Status != "working" || st.Sessions[0].Project.Kind != grouping.KindDir {
				t.Fatalf("state %+v", st)
			}
		}},
		{"same snapshot", snapshot("working"), nil, false, nil},
		{"status flips", snapshot("waiting"), nil, true, func(t *testing.T, st State) {
			if st.Sessions[0].Status != "waiting" {
				t.Fatalf("status %q", st.Sessions[0].Status)
			}
		}},
		{"failure keeps last list", am.Snapshot{}, errors.New("boom"), true, func(t *testing.T, st State) {
			if st.Error == nil || st.Error.Kind != ErrFailed || len(st.Sessions) != 1 {
				t.Fatalf("state %+v", st)
			}
		}},
		{"not installed", am.Snapshot{}, am.ErrNotInstalled, true, func(t *testing.T, st State) {
			if st.Error == nil || st.Error.Kind != ErrNotInstalled {
				t.Fatalf("state %+v", st)
			}
		}},
		{"recovery clears error", snapshot("waiting"), nil, true, func(t *testing.T, st State) {
			if st.Error != nil {
				t.Fatalf("error not cleared: %+v", st.Error)
			}
		}},
	}
	for _, step := range steps {
		c.set(step.snap, step.err)
		p.Poll(context.Background())
		select {
		case st := <-ch:
			if !step.wantPublish {
				t.Fatalf("%s: unexpected publish", step.name)
			}
			step.check(t, st)
		default:
			if step.wantPublish {
				t.Fatalf("%s: nothing published", step.name)
			}
		}
	}
}

func TestInsideSession(t *testing.T) {
	snap := snapshot("idle")
	snap.Sessions[0].Self = true
	p := newPoller(&fakeClient{snap: snap})
	p.Poll(context.Background())
	if !p.State().InsideSession {
		t.Fatal("InsideSession not set")
	}
}

func TestSlowSubscriberGetsNewest(t *testing.T) {
	c := &fakeClient{}
	p := newPoller(c)
	ch, cancel := p.Subscribe()
	defer cancel()
	for _, st := range []string{"working", "waiting", "finished"} {
		c.set(snapshot(st), nil)
		p.Poll(context.Background())
	}
	if got := (<-ch).Sessions[0].Status; got != "finished" {
		t.Fatalf("got %q, want newest", got)
	}
}

func TestHookWriteTriggersPoll(t *testing.T) {
	dir := t.TempDir()
	c := &fakeClient{snap: snapshot("working")}
	p := newPoller(c)
	p.HooksDir = dir
	p.Interval = time.Hour
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go p.Run(ctx)

	waitFor(t, func() bool { return c.calls.Load() >= 1 })
	before := c.calls.Load()
	if err := os.WriteFile(filepath.Join(dir, "a1b2c3d4.status"), []byte("waiting"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return c.calls.Load() > before })

	before = c.calls.Load()
	if err := os.WriteFile(filepath.Join(dir, "a1b2c3d4.agent"), []byte("1"), 0o600); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if c.calls.Load() != before {
		t.Fatal("non-status file triggered a poll")
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
