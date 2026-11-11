// Package send delivers a human reply to an agent session through
// agent-manager, refusing when the text would land somewhere other than
// the agent's prompt.
package send

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gsperry/agent-desk/internal/hub"
)

type Reason string

const (
	ReasonUnknownSession Reason = "unknown_session"
	ReasonNotRunning     Reason = "not_running"
	ReasonStarting       Reason = "starting"
	ReasonWaiting        Reason = "waiting"
	ReasonDialog         Reason = "dialog"
	ReasonDraft          Reason = "draft"
	ReasonInsideSession  Reason = "inside_session"
	ReasonEmpty          Reason = "empty"
	ReasonTooLong        Reason = "too_long"
	ReasonBusy           Reason = "busy"
)

// Refused is a send the gate stopped before anything reached the pane.
type Refused struct {
	Reason  Reason `json:"reason"`
	Message string `json:"message"`
}

func (r *Refused) Error() string { return r.Message }

// MaxBytes caps one reply. agent-manager pastes through a tmux buffer, which
// has no small limit; this keeps an accidental paste of a huge file out.
const MaxBytes = 256 << 10

// Clean normalizes line endings and removes control characters other than
// newline and tab. ESC matters most: text holding the bracketed-paste end
// sequence would close the paste early and arrive as keystrokes.
func Clean(text string) (string, *Refused) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if r == utf8.RuneError || unicode.IsControl(r) {
			return -1
		}
		return r
	}, text)
	text = strings.TrimSpace(text)
	if text == "" {
		return "", &Refused{ReasonEmpty, "The message is empty."}
	}
	if len(text) > MaxBytes {
		return "", &Refused{ReasonTooLong, "The message is over 256 KB. Put the detail in a file and point the agent at it."}
	}
	return text, nil
}

// CheckStatus refuses statuses where typed text would not reach the
// agent's prompt.
func CheckStatus(s *hub.SessionView, insideSession bool) *Refused {
	switch {
	case s == nil:
		return &Refused{ReasonUnknownSession, "This session no longer exists in agent-manager."}
	case insideSession:
		return &Refused{ReasonInsideSession, "Agent Desk is running inside an agent-manager session, where agent-manager refuses to type for you. Start it from a regular terminal."}
	case !s.Running || s.Status == "dead":
		return &Refused{ReasonNotRunning, "This session is not running. Revive it in agent-manager first."}
	case s.Status == "starting":
		return &Refused{ReasonStarting, "The agent is still starting. Try again in a moment."}
	case s.Status == "waiting":
		return &Refused{ReasonWaiting, "The agent is waiting on a question or permission prompt in its terminal. Text sent now would answer it. Answer it in agent-manager."}
	}
	return nil
}

var (
	dialogFooter = regexp.MustCompile(`Enter to (?:confirm|select)|Esc to cancel`)
	ruleLine     = regexp.MustCompile(`^\s*─{20,}\s*$`)
)

// footerLines is how far up from the bottom a dialog footer is looked for.
// Dialogs draw it at or near the bottom; agent output quoting the same
// words sits higher up.
const footerLines = 8

// CheckPane refuses when the visible screen shows a dialog, or text already
// typed into the agent's own composer, which a paste would append to.
func CheckPane(screen string) *Refused {
	var lines []string
	for _, l := range strings.Split(screen, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	for _, l := range lines[max(len(lines)-footerLines, 0):] {
		if dialogFooter.MatchString(l) {
			return &Refused{ReasonDialog, "The session is showing a dialog. Text sent now would pick an option. Answer it in agent-manager."}
		}
	}
	// The composer sits between the last two horizontal rules.
	var rules []int
	for i, l := range lines {
		if ruleLine.MatchString(l) {
			rules = append(rules, i)
		}
	}
	if len(rules) >= 2 {
		for _, l := range lines[rules[len(rules)-2]+1 : rules[len(rules)-1]] {
			if isDraft(l) {
				return &Refused{ReasonDraft, "There is unsent text in the session's own prompt. Sending would append to it. Clear or send it in agent-manager first."}
			}
		}
	}
	return nil
}

// isDraft reports a composer line holding text. Claude pads the prompt
// marker with a no-break space, so trimming must be Unicode-aware.
func isDraft(line string) bool {
	rest, ok := strings.CutPrefix(strings.TrimLeftFunc(line, unicode.IsSpace), "❯")
	return ok && strings.TrimFunc(rest, unicode.IsSpace) != ""
}
