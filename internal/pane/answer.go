package pane

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/gsperry/agent-desk/internal/hub"
	"github.com/gsperry/agent-desk/internal/run"
	"github.com/gsperry/agent-desk/internal/send"
)

// AllowedKeys are the only keys Agent Desk sends to an agent's pane.
var AllowedKeys = map[string]bool{"Up": true, "Down": true, "Enter": true, "Escape": true}

var validID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Keys presses named keys in a session's agent pane.
type Keys interface {
	Send(ctx context.Context, id string, keys ...string) error
}

// TmuxKeys sends keys through agent-manager's tmux server. Every call names
// the agentmgr socket and pane 0, which agent-manager keeps as the agent.
type TmuxKeys struct {
	Runner run.Runner
}

func (k TmuxKeys) Send(ctx context.Context, id string, keys ...string) error {
	if err := checkKeys(id, keys); err != nil {
		return err
	}
	args := append([]string{"-L", "agentmgr", "send-keys", "-t", "am_" + id + ".0"}, keys...)
	_, err := k.Runner.Run(ctx, "tmux", args...)
	return err
}

// FileKeys records key presses in a file, for fixture mode.
type FileKeys struct {
	Path string
}

func (k FileKeys) Send(_ context.Context, id string, keys ...string) error {
	if err := checkKeys(id, keys); err != nil {
		return err
	}
	f, err := os.OpenFile(k.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s %s\n", id, strings.Join(keys, " "))
	return err
}

func checkKeys(id string, keys []string) error {
	if !validID.MatchString(id) {
		return fmt.Errorf("invalid session id %q", id)
	}
	for _, key := range keys {
		if !AllowedKeys[key] {
			return fmt.Errorf("key %q is not allowed", key)
		}
	}
	return nil
}

type Sessions interface {
	State() hub.State
	Trigger()
}

type Reader interface {
	Read(ctx context.Context, id string) (string, error)
	ReadStyled(ctx context.Context, id string) (string, error)
}

// Screen is what the live pane shows.
type Screen struct {
	Text   string  `json:"text"`
	Dialog *Dialog `json:"dialog,omitempty"`
}

// Answer is a reply to a dialog: choose an option by index, confirming its
// label still matches, or press one allowed key.
type Answer struct {
	Action string `json:"action"`
	Index  int    `json:"index"`
	Label  string `json:"label"`
	Key    string `json:"key"`
}

const (
	reasonNoDialog send.Reason = "no_dialog"
	reasonChanged  send.Reason = "changed"
	reasonFree     send.Reason = "free_option"
	reasonBadKey   send.Reason = "bad_key"
)

type Answerer struct {
	Sessions Sessions
	Reader   Reader
	Keys     Keys
}

// screenLines is how much of the screen the live pane shows.
const screenLines = 40

func (a *Answerer) Screen(ctx context.Context, id string) (Screen, error) {
	if refused := a.check(id); refused != nil {
		return Screen{}, refused
	}
	text, err := a.Reader.Read(ctx, id)
	if err != nil {
		return Screen{}, err
	}
	return Screen{Text: tail(text, screenLines), Dialog: Parse(text)}, nil
}

// Styled returns the whole screen with its color escape sequences, for a
// read-only terminal view.
func (a *Answerer) Styled(ctx context.Context, id string) (string, error) {
	if refused := a.check(id); refused != nil {
		return "", refused
	}
	return a.Reader.ReadStyled(ctx, id)
}

func (a *Answerer) Answer(ctx context.Context, id string, ans Answer) error {
	if refused := a.check(id); refused != nil {
		return refused
	}
	var keys []string
	switch ans.Action {
	case "key":
		if !AllowedKeys[ans.Key] {
			return &send.Refused{Reason: reasonBadKey, Message: fmt.Sprintf("Key %q is not one Agent Desk sends.", ans.Key)}
		}
		keys = []string{ans.Key}
	case "choose":
		// The screen is read again so a dialog that changed since the
		// reader saw it is not answered with the wrong option.
		text, err := a.Reader.Read(ctx, id)
		if err != nil {
			return fmt.Errorf("could not read the session's screen, so nothing was pressed: %w", err)
		}
		d := Parse(text)
		if d == nil || len(d.Options) == 0 {
			return &send.Refused{Reason: reasonNoDialog, Message: "The session is no longer showing a choice."}
		}
		if ans.Index < 0 || ans.Index >= len(d.Options) || d.Options[ans.Index].Label != ans.Label {
			return &send.Refused{Reason: reasonChanged, Message: "The choice on screen changed. Check it again before answering."}
		}
		if d.Options[ans.Index].Free {
			return &send.Refused{Reason: reasonFree, Message: "That option needs typing in the terminal. Answer it in agent-manager."}
		}
		keys = d.Moves(ans.Index)
	default:
		return &send.Refused{Reason: reasonBadKey, Message: "Unknown answer."}
	}
	err := a.Keys.Send(ctx, id, keys...)
	a.Sessions.Trigger()
	return err
}

func (a *Answerer) check(id string) *send.Refused {
	for _, s := range a.Sessions.State().Sessions {
		if s.ID != id {
			continue
		}
		if !s.Running {
			return &send.Refused{Reason: send.ReasonNotRunning, Message: "This session is not running."}
		}
		return nil
	}
	return &send.Refused{Reason: send.ReasonUnknownSession, Message: "This session no longer exists in agent-manager."}
}

func tail(text string, n int) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
