package pane

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode"

	"github.com/gsperry/agent-desk/internal/hub"
	"github.com/gsperry/agent-desk/internal/run"
	"github.com/gsperry/agent-desk/internal/send"
)

// AllowedKeys are the only keys Agent Desk sends to an agent's pane. Left
// and Right move between the questions of a multi-question prompt, and are
// sent only by a step answer.
var AllowedKeys = map[string]bool{"Up": true, "Down": true, "Enter": true, "Escape": true, "Left": true, "Right": true}

// pressKeys are the keys a key answer may press.
var pressKeys = map[string]bool{"Up": true, "Down": true, "Enter": true, "Escape": true}

// stepKeys move to the previous or next question.
var stepKeys = map[string]bool{"Left": true, "Right": true}

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
// label still matches; answer a text option with text; press one allowed
// key; or step to the previous or next question (Key Left or Right).
type Answer struct {
	Action string `json:"action"`
	Index  int    `json:"index"`
	Label  string `json:"label"`
	Key    string `json:"key"`
	Text   string `json:"text,omitempty"`
}

// Typer submits text in a session's pane. It is agent-manager type.
type Typer interface {
	Type(ctx context.Context, id, text string) error
}

// maxAnswerText bounds a typed answer.
const maxAnswerText = 2000

const (
	reasonNoDialog send.Reason = "no_dialog"
	reasonChanged  send.Reason = "changed"
	reasonFree     send.Reason = "free_option"
	reasonBadKey   send.Reason = "bad_key"
	reasonBadText  send.Reason = "bad_text"
	reasonNotBusy  send.Reason = "not_working"
	reasonAsking   send.Reason = "asking"
)

type Answerer struct {
	Sessions Sessions
	Reader   Reader
	Keys     Keys
	// Typer enters text answers; nil refuses them.
	Typer Typer
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
		if !pressKeys[ans.Key] {
			return &send.Refused{Reason: reasonBadKey, Message: fmt.Sprintf("Key %q is not one Agent Desk sends.", ans.Key)}
		}
		// A key answers a question on screen; with none, Escape would
		// interrupt the agent, which is Interrupt's job and level.
		text, err := a.Reader.Read(ctx, id)
		if err != nil {
			return fmt.Errorf("could not read the session's screen, so nothing was pressed: %w", err)
		}
		if Parse(text) == nil {
			return &send.Refused{Reason: reasonNoDialog, Message: "The session is no longer showing a question."}
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
			return &send.Refused{Reason: reasonFree, Message: "That option takes text. Type your answer and send it."}
		}
		keys = d.Moves(ans.Index)
	case "step":
		if !stepKeys[ans.Key] {
			return &send.Refused{Reason: reasonBadKey, Message: fmt.Sprintf("Key %q does not move between questions.", ans.Key)}
		}
		text, err := a.Reader.Read(ctx, id)
		if err != nil {
			return fmt.Errorf("could not read the session's screen, so nothing was pressed: %w", err)
		}
		d := Parse(text)
		if d == nil || len(d.Steps) < 2 {
			return &send.Refused{Reason: reasonNoDialog, Message: "The session is no longer showing a prompt with several questions."}
		}
		keys = d.stepKeys(ans.Key)
	case "text":
		err := a.answerText(ctx, id, ans)
		a.Sessions.Trigger()
		return err
	default:
		return &send.Refused{Reason: reasonBadKey, Message: "Unknown answer."}
	}
	err := a.Keys.Send(ctx, id, keys...)
	a.Sessions.Trigger()
	return err
}

// answerText answers a text option: it moves the selection onto the option,
// which makes it Claude Code's text field, confirms on a fresh screen that
// the field is selected, then submits the text with agent-manager type.
// Each step reads the screen again and stops if the dialog changed.
func (a *Answerer) answerText(ctx context.Context, id string, ans Answer) error {
	if a.Typer == nil {
		return &send.Refused{Reason: reasonBadText, Message: "Typed answers are not available."}
	}
	text := strings.TrimSpace(ans.Text)
	switch {
	case text == "":
		return &send.Refused{Reason: reasonBadText, Message: "Type an answer first."}
	case len(text) > maxAnswerText:
		return &send.Refused{Reason: reasonBadText, Message: fmt.Sprintf("Answers are limited to %d characters.", maxAnswerText)}
	case strings.ContainsFunc(text, unicode.IsControl):
		return &send.Refused{Reason: reasonBadText, Message: "A typed answer is one line, without control characters."}
	}
	d, err := a.dialog(ctx, id)
	if err != nil {
		return err
	}
	if ans.Index < 0 || ans.Index >= len(d.Options) || d.Options[ans.Index].Label != ans.Label || !d.Options[ans.Index].Free {
		return &send.Refused{Reason: reasonChanged, Message: "The choice on screen changed. Check it again before answering."}
	}
	moves := d.Moves(ans.Index)
	if moves = moves[:len(moves)-1]; len(moves) > 0 {
		if err := a.Keys.Send(ctx, id, moves...); err != nil {
			return err
		}
	}
	after, err := a.dialog(ctx, id)
	if err != nil {
		return err
	}
	if ans.Index >= len(after.Options) || !after.Options[ans.Index].Selected || !after.Options[ans.Index].Free || after.Options[ans.Index].Label != ans.Label {
		return &send.Refused{Reason: reasonChanged, Message: "The text field did not become active, so nothing was typed. Check the terminal."}
	}
	return a.Typer.Type(ctx, id, text)
}

// dialog reads the screen and parses the dialog on it.
func (a *Answerer) dialog(ctx context.Context, id string) (*Dialog, error) {
	text, err := a.Reader.Read(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("could not read the session's screen, so nothing was entered: %w", err)
	}
	d := Parse(text)
	if d == nil || len(d.Options) == 0 {
		return nil, &send.Refused{Reason: reasonNoDialog, Message: "The session is no longer showing a choice."}
	}
	return d, nil
}

// Interrupt presses Escape in a working session, as Esc does in Claude
// Code: the agent stops its current turn and waits for a prompt. It is
// refused unless the session is working, and while a question is on
// screen, which Escape would cancel instead.
func (a *Answerer) Interrupt(ctx context.Context, id string) error {
	if refused := a.check(id); refused != nil {
		return refused
	}
	for _, s := range a.Sessions.State().Sessions {
		if s.ID == id && s.Status != "working" {
			return &send.Refused{Reason: reasonNotBusy, Message: "The session is not working, so there is nothing to interrupt."}
		}
	}
	text, err := a.Reader.Read(ctx, id)
	if err != nil {
		return fmt.Errorf("could not read the session's screen, so nothing was pressed: %w", err)
	}
	if Parse(text) != nil {
		return &send.Refused{Reason: reasonAsking, Message: "The session is asking a question. Answer or cancel it instead."}
	}
	err = a.Keys.Send(ctx, id, "Escape")
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
