package send

import "sync"

// Locks lets one flow at a time drive a session's terminal. Replies,
// answers, interrupts and the model and subagent flows share one, so their
// key presses and pastes never interleave.
type Locks struct {
	mu   sync.Mutex
	held map[string]bool
}

// Claim takes the session's lock, or reports false when another flow holds
// it.
func (l *Locks) Claim(id string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.held == nil {
		l.held = map[string]bool{}
	}
	if l.held[id] {
		return false
	}
	l.held[id] = true
	return true
}

func (l *Locks) Release(id string) {
	l.mu.Lock()
	delete(l.held, id)
	l.mu.Unlock()
}

// Busy is the refusal while another flow holds a session's lock.
func Busy() *Refused {
	return &Refused{ReasonBusy, "Agent Desk is still working in this session's terminal. Try again in a moment."}
}
