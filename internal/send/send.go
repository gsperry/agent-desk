package send

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/gsperry/agent-desk/internal/am"
	"github.com/gsperry/agent-desk/internal/hub"
)

type Sessions interface {
	State() hub.State
	Trigger()
}

type Pane interface {
	Type(ctx context.Context, id, text string) error
	Read(ctx context.Context, id string) (string, error)
}

// Result is a send that reached agent-manager. Maybe means the paste started
// before a failure, so the text may be in the pane; the transcript decides.
type Result struct {
	Status string `json:"status"`
}

const (
	StatusSent  = "sent"
	StatusMaybe = "maybe"
)

type Sender struct {
	Sessions Sessions
	Pane     Pane

	mu       sync.Mutex
	inFlight map[string]bool
}

// Send checks the session and its screen, then types the text. A *Refused
// error means nothing reached the pane.
func (s *Sender) Send(ctx context.Context, id, text string) (Result, error) {
	text, refused := Clean(text)
	if refused != nil {
		return Result{}, refused
	}
	if !s.claim(id) {
		return Result{}, &Refused{ReasonBusy, "A message to this session is still being sent."}
	}
	defer s.release(id)

	st := s.Sessions.State()
	var session *hub.SessionView
	for i := range st.Sessions {
		if st.Sessions[i].ID == id {
			session = &st.Sessions[i]
		}
	}
	if refused := CheckStatus(session, st.InsideSession); refused != nil {
		return Result{}, refused
	}
	// The status can be two seconds old, and some dialogs never read as
	// waiting, so the screen itself is checked last.
	screen, err := s.Pane.Read(ctx, id)
	if err != nil {
		return Result{}, fmt.Errorf("could not read the session's screen to check for a dialog, so nothing was sent: %w", err)
	}
	if refused := CheckPane(screen); refused != nil {
		return Result{}, refused
	}
	if refused := CheckWaiting(session.Status, screen); refused != nil {
		return Result{}, refused
	}
	err = s.Pane.Type(ctx, id, text)
	s.Sessions.Trigger()
	switch {
	case errors.Is(err, am.ErrMaybeSent):
		return Result{Status: StatusMaybe}, nil
	case err != nil:
		return Result{}, err
	}
	return Result{Status: StatusSent}, nil
}

func (s *Sender) claim(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inFlight == nil {
		s.inFlight = map[string]bool{}
	}
	if s.inFlight[id] {
		return false
	}
	s.inFlight[id] = true
	return true
}

func (s *Sender) release(id string) {
	s.mu.Lock()
	delete(s.inFlight, id)
	s.mu.Unlock()
}
