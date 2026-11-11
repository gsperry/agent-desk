package send

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/gsperry/agent-desk/internal/am"
	"github.com/gsperry/agent-desk/internal/hub"
)

type fakeSessions struct {
	st       hub.State
	triggers int
}

func (f *fakeSessions) State() hub.State { return f.st }
func (f *fakeSessions) Trigger()         { f.triggers++ }

type fakePane struct {
	screen  string
	readErr error
	typeErr error
	typed   []string
	block   chan struct{}
}

func (f *fakePane) Read(context.Context, string) (string, error) { return f.screen, f.readErr }
func (f *fakePane) Type(_ context.Context, _ string, text string) error {
	if f.block != nil {
		<-f.block
	}
	f.typed = append(f.typed, text)
	return f.typeErr
}

func newSender(status string, pane *fakePane) (*Sender, *fakeSessions) {
	ss := &fakeSessions{st: hub.State{Sessions: []hub.SessionView{{ID: "s1", Status: status, Running: true}}}}
	return &Sender{Sessions: ss, Pane: pane}, ss
}

func TestSend(t *testing.T) {
	tests := []struct {
		name       string
		status     string
		pane       fakePane
		text       string
		wantStatus string
		wantReason Reason
		wantErr    bool
		wantTyped  bool
	}{
		{"sent", "idle", fakePane{screen: idleScreen}, " hi \r\nthere ", StatusSent, "", false, true},
		{"maybe sent", "idle", fakePane{screen: idleScreen, typeErr: fmt.Errorf("%w: x", am.ErrMaybeSent)}, "hi", StatusMaybe, "", false, true},
		{"type failed", "idle", fakePane{screen: idleScreen, typeErr: errors.New("boom")}, "hi", "", "", true, true},
		{"waiting status", "waiting", fakePane{screen: idleScreen}, "hi", "", ReasonWaiting, false, false},
		{"dialog on screen", "finished", fakePane{screen: "x\nEnter to confirm · Esc to cancel\n"}, "hi", "", ReasonDialog, false, false},
		{"unreadable screen", "idle", fakePane{readErr: errors.New("tmux gone")}, "hi", "", "", true, false},
		{"empty text", "idle", fakePane{screen: idleScreen}, "  ", "", ReasonEmpty, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pane := tt.pane
			s, ss := newSender(tt.status, &pane)
			res, err := s.Send(context.Background(), "s1", tt.text)
			var refused *Refused
			switch {
			case tt.wantReason != "":
				if !errors.As(err, &refused) || refused.Reason != tt.wantReason {
					t.Fatalf("err = %v, want refusal %s", err, tt.wantReason)
				}
			case tt.wantErr:
				if err == nil || errors.As(err, &refused) {
					t.Fatalf("err = %v, want a delivery error", err)
				}
			default:
				if err != nil || res.Status != tt.wantStatus {
					t.Fatalf("res = %+v, err = %v; want %s", res, err, tt.wantStatus)
				}
			}
			if (len(pane.typed) > 0) != tt.wantTyped {
				t.Fatalf("typed = %q, want typed=%v", pane.typed, tt.wantTyped)
			}
			if tt.wantTyped && ss.triggers != 1 {
				t.Fatalf("triggers = %d", ss.triggers)
			}
		})
	}
}

func TestSendCleansText(t *testing.T) {
	pane := &fakePane{screen: idleScreen}
	s, _ := newSender("idle", pane)
	if _, err := s.Send(context.Background(), "s1", "a\r\nb\x1b[201~"); err != nil {
		t.Fatal(err)
	}
	if pane.typed[0] != "a\nb[201~" {
		t.Fatalf("typed %q", pane.typed[0])
	}
}

func TestConcurrentSendIsRefused(t *testing.T) {
	pane := &fakePane{screen: idleScreen, block: make(chan struct{})}
	s, _ := newSender("idle", pane)
	done := make(chan error)
	go func() {
		_, err := s.Send(context.Background(), "s1", "first")
		done <- err
	}()
	for !s.busy("s1") {
	}
	_, err := s.Send(context.Background(), "s1", "second")
	var refused *Refused
	if !errors.As(err, &refused) || refused.Reason != ReasonBusy {
		t.Fatalf("second send: %v", err)
	}
	close(pane.block)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

const idleScreen = "────────────────────────────────────────\n❯ \n────────────────────────────────────────\n"

func (s *Sender) busy(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inFlight[id]
}
