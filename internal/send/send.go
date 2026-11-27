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

// StyledPane also reads the screen with its colors, which tells text the
// user typed from Claude Code's dim placeholder or suggested prompt.
type StyledPane interface {
	ReadStyled(ctx context.Context, id string) (string, error)
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
	// Locks is shared with every other flow that drives a session's
	// terminal; nil keeps one of its own.
	Locks *Locks

	once sync.Once
}

// Send checks the session and its screen, then types the text. A *Refused
// error means nothing reached the pane.
func (s *Sender) Send(ctx context.Context, id, text string) (Result, error) {
	text, refused := Clean(text)
	if refused != nil {
		return Result{}, refused
	}
	locks := s.locks()
	if !locks.Claim(id) {
		return Result{}, &Refused{ReasonBusy, "A message to this session is still being sent, or Agent Desk is answering it. Try again in a moment."}
	}
	defer locks.Release(id)

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
	if refused := CheckPane(screen); refused != nil && !s.standIn(ctx, id, refused) {
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

// standIn reports a draft refusal that is only Claude Code's dim stand-in
// text in the prompt: a placeholder, or a suggested next prompt it shows
// after some turns. A reply replaces that text, so it is not refused.
func (s *Sender) standIn(ctx context.Context, id string, refused *Refused) bool {
	p, ok := s.Pane.(StyledPane)
	if !ok || refused.Reason != ReasonDraft {
		return false
	}
	styled, err := p.ReadStyled(ctx, id)
	return err == nil && StandIn(styled)
}

func (s *Sender) locks() *Locks {
	s.once.Do(func() {
		if s.Locks == nil {
			s.Locks = &Locks{}
		}
	})
	return s.Locks
}
