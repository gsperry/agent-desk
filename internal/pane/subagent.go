package pane

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gsperry/agent-desk/internal/send"
)

// AgentTarget names a subagent by what Claude Code shows of it: its type
// in the agent list, and its description in the rule above its prompt.
type AgentTarget struct {
	Type        string
	Description string
}

const (
	reasonNoList    send.Reason = "no_agent_list"
	reasonNotListed send.Reason = "agent_not_listed"
	reasonLost      send.Reason = "agent_view_lost"
)

// maxMoves bounds the arrow presses to reach a row.
const maxMoves = 20

// MessageAgent types a message to a running subagent the way Claude Code's
// own screen does: Down opens the agent list, Down moves to the subagent,
// Enter shows its conversation, and the text goes into its message box
// through agent-manager type. Down and Enter on main bring the screen
// back. Each step reads the screen again and stops if it is not as
// expected; Escape is never pressed while a subagent is shown, since there
// it interrupts the subagent. The text reaches the subagent at its next
// step, as Claude Code queues it.
func (a *Answerer) MessageAgent(ctx context.Context, id string, t AgentTarget, text string) error {
	if refused := a.check(id); refused != nil {
		return refused
	}
	release, busy := a.lock(id)
	if busy != nil {
		return busy
	}
	defer release()
	if a.Typer == nil {
		return &send.Refused{Reason: reasonBadText, Message: "Typed messages are not available."}
	}
	text, refused := send.Clean(text)
	if refused != nil {
		return refused
	}
	l, err := a.agentList(ctx, id)
	if err != nil {
		return err
	}
	if !l.PromptEmpty {
		return &send.Refused{Reason: send.ReasonDraft, Message: "There is unsent text in the session's own prompt. Clear or send it in agent-manager first."}
	}
	rows := l.candidates(t.Type, t.Description)
	if len(rows) == 0 {
		return &send.Refused{Reason: reasonNotListed, Message: "Claude Code is not listing this subagent, so it cannot take a message. It may have finished."}
	}
	for _, row := range rows {
		found, err := a.viewAgent(ctx, id, row, t)
		if err != nil {
			return a.backToMain(ctx, id, err)
		}
		if !found {
			continue
		}
		if err := a.Typer.Type(ctx, id, text); err != nil {
			return a.backToMain(ctx, id, err)
		}
		after, err := a.agentList(ctx, id)
		if err != nil {
			return a.backToMain(ctx, id, err)
		}
		if !after.Viewing || after.Title != t.Description || !after.PromptEmpty {
			return a.backToMain(ctx, id, &send.Refused{Reason: reasonLost, Message: "The message was typed, but the screen did not show it sent to the subagent. Check the session in agent-manager."})
		}
		return a.backToMain(ctx, id, nil)
	}
	return a.backToMain(ctx, id, &send.Refused{Reason: reasonNotListed, Message: fmt.Sprintf("Claude Code's agent list has no subagent described %q.", t.Description)})
}

// agentList reads the screen, refusing when a dialog is on it or there is
// no agent list.
func (a *Answerer) agentList(ctx context.Context, id string) (*AgentList, error) {
	text, err := a.Reader.Read(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("could not read the session's screen, so nothing was pressed: %w", err)
	}
	if Parse(text) != nil {
		return nil, &send.Refused{Reason: reasonAsking, Message: "The session is asking a question. Answer it first."}
	}
	l := ParseAgents(text)
	if l == nil {
		return nil, &send.Refused{Reason: reasonNoList, Message: "Claude Code is not showing its agent list, so the subagent cannot be reached. It may have finished."}
	}
	return l, nil
}

// selectRow opens the agent list and moves its selection to row.
func (a *Answerer) selectRow(ctx context.Context, id string, row int) (*AgentList, error) {
	l, err := a.agentList(ctx, id)
	if err != nil {
		return nil, err
	}
	if !l.Selecting {
		if !l.PromptEmpty {
			return nil, &send.Refused{Reason: send.ReasonDraft, Message: "There is unsent text in the session's prompt."}
		}
		// Down from an empty prompt opens the list on its first row.
		if err := a.Keys.Send(ctx, id, "Down"); err != nil {
			return nil, err
		}
		if l, err = a.agentList(ctx, id); err != nil {
			return nil, err
		}
		if !l.Selecting {
			return nil, &send.Refused{Reason: reasonLost, Message: "Claude Code's agent list did not open."}
		}
	}
	if row >= len(l.Rows) || abs(row-l.Cursor) > maxMoves {
		return nil, &send.Refused{Reason: reasonLost, Message: "The agent list changed."}
	}
	var keys []string
	for i := l.Cursor; i < row; i++ {
		keys = append(keys, "Down")
	}
	for i := l.Cursor; i > row; i-- {
		keys = append(keys, "Up")
	}
	if len(keys) > 0 {
		if err := a.Keys.Send(ctx, id, keys...); err != nil {
			return nil, err
		}
		if l, err = a.agentList(ctx, id); err != nil {
			return nil, err
		}
	}
	if !l.Selecting || l.Cursor != row {
		return nil, &send.Refused{Reason: reasonLost, Message: "The agent list changed while Agent Desk moved through it."}
	}
	return l, nil
}

// viewAgent shows row's conversation and reports whether it is the
// subagent t, by the description in the rule above its prompt.
func (a *Answerer) viewAgent(ctx context.Context, id string, row int, t AgentTarget) (bool, error) {
	l, err := a.selectRow(ctx, id, row)
	if err != nil {
		return false, err
	}
	if !l.Rows[row].matchesType(t.Type) {
		return false, &send.Refused{Reason: reasonLost, Message: "The agent list changed."}
	}
	if err := a.Keys.Send(ctx, id, "Enter"); err != nil {
		return false, err
	}
	if l, err = a.agentList(ctx, id); err != nil {
		return false, err
	}
	if !l.Viewing || l.Shown != row {
		return false, &send.Refused{Reason: reasonLost, Message: "Claude Code did not show the subagent's conversation."}
	}
	return l.Title == t.Description, nil
}

// backToMain shows the main conversation again and closes the agent list,
// then returns cause. When the screen cannot be brought back, it says so,
// since replies to the session would reach the subagent.
func (a *Answerer) backToMain(ctx context.Context, id string, cause error) error {
	if err := a.restoreMain(ctx, id); err != nil {
		var r *send.Refused
		switch {
		case cause == nil:
			return err
		case errors.As(cause, &r):
			return &send.Refused{Reason: r.Reason, Message: r.Message + " Also, " + err.Error() + "."}
		default:
			return fmt.Errorf("%w; also, %v", cause, err)
		}
	}
	return cause
}

func (a *Answerer) restoreMain(ctx context.Context, id string) error {
	text, err := a.Reader.Read(ctx, id)
	if err != nil {
		return err
	}
	l := ParseAgents(text)
	if l == nil || Parse(text) != nil {
		// No list to work in, or a dialog that keys would answer.
		return nil
	}
	if l.Viewing || l.Shown != 0 {
		if _, err := a.selectRow(ctx, id, 0); err != nil {
			return lostMain()
		}
		if err := a.Keys.Send(ctx, id, "Enter"); err != nil {
			return err
		}
		if l, err = a.agentList(ctx, id); err != nil || l.Viewing || l.Shown != 0 {
			return lostMain()
		}
	}
	// Up from the first row closes the list.
	if l.Selecting {
		if l.Cursor > 0 {
			if _, err := a.selectRow(ctx, id, 0); err != nil {
				return lostMain()
			}
		}
		if err := a.Keys.Send(ctx, id, "Up"); err != nil {
			return err
		}
		if l, err = a.agentList(ctx, id); err != nil || l.Selecting {
			return lostMain()
		}
	}
	return nil
}

func lostMain() error {
	return &send.Refused{Reason: reasonLost, Message: "the session's screen may still show the subagent: switch it back to main in agent-manager before replying"}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// StopAgent stops a running subagent as Claude Code's agent list does: it
// confirms which row is the subagent by showing it, as MessageAgent does,
// brings the screen back to main, selects that row again, and presses x
// only while the list's hint offers "x to stop". It then checks the hint
// changed to "x to clear" and closes the list. The subagent's work so far
// stays; the main agent is told it was stopped.
func (a *Answerer) StopAgent(ctx context.Context, id string, t AgentTarget) error {
	if refused := a.check(id); refused != nil {
		return refused
	}
	release, busy := a.lock(id)
	if busy != nil {
		return busy
	}
	defer release()
	l, err := a.agentList(ctx, id)
	if err != nil {
		return err
	}
	if !l.PromptEmpty {
		return &send.Refused{Reason: send.ReasonDraft, Message: "There is unsent text in the session's own prompt. Clear or send it in agent-manager first."}
	}
	rows := l.candidates(t.Type, t.Description)
	if len(rows) == 0 {
		return &send.Refused{Reason: reasonNotListed, Message: "Claude Code is not listing this subagent. It may have finished already."}
	}
	for _, row := range rows {
		found, err := a.viewAgent(ctx, id, row, t)
		if err != nil {
			return a.backToMain(ctx, id, err)
		}
		if !found {
			continue
		}
		if err := a.restoreMain(ctx, id); err != nil {
			return err
		}
		return a.backToMain(ctx, id, a.pressStop(ctx, id, row, t))
	}
	return a.backToMain(ctx, id, &send.Refused{Reason: reasonNotListed, Message: fmt.Sprintf("Claude Code's agent list has no subagent described %q.", t.Description)})
}

// pressStop selects row from the main conversation and presses x.
func (a *Answerer) pressStop(ctx context.Context, id string, row int, t AgentTarget) error {
	l, err := a.selectRow(ctx, id, row)
	if err != nil {
		return err
	}
	if l.Viewing || l.Shown != 0 || !l.Rows[row].matchesType(t.Type) {
		return &send.Refused{Reason: reasonLost, Message: "The agent list changed."}
	}
	if !strings.Contains(l.Hint, "x to stop") {
		return &send.Refused{Reason: reasonNotListed, Message: "Claude Code does not offer to stop this subagent. It may have finished or been stopped already."}
	}
	if err := a.Keys.Send(ctx, id, "x"); err != nil {
		return err
	}
	after, err := a.agentList(ctx, id)
	if err != nil {
		return err
	}
	if after.Cursor == row && strings.Contains(after.Hint, "x to stop") {
		return &send.Refused{Reason: reasonLost, Message: "Claude Code still offers to stop the subagent after x was pressed. Check the session in agent-manager."}
	}
	return nil
}
