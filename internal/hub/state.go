// Package hub keeps the live session list and fans changes out to clients.
package hub

import (
	"github.com/gsperry/agent-desk/internal/am"
	"github.com/gsperry/agent-desk/internal/grouping"
)

// State is what the sidebar renders. It is published whole on every change.
type State struct {
	ManagerAwake bool `json:"managerAwake"`
	// InsideSession is set when Agent Desk itself runs inside an agent-manager
	// pane, where agent-manager refuses to type into other sessions.
	InsideSession bool          `json:"insideSession"`
	Error         *StateError   `json:"error,omitempty"`
	Sessions      []SessionView `json:"sessions"`
}

type ErrorKind string

const (
	ErrNotInstalled ErrorKind = "not_installed"
	ErrFailed       ErrorKind = "failed"
)

type StateError struct {
	Kind    ErrorKind `json:"kind"`
	Message string    `json:"message"`
}

type SessionView struct {
	ID             string           `json:"id"`
	Name           string           `json:"name"`
	Tool           string           `json:"tool"`
	Group          string           `json:"group"`
	Directory      string           `json:"directory"`
	Status         string           `json:"status"`
	Running        bool             `json:"running"`
	Archived       bool             `json:"archived"`
	Branch         string           `json:"branch,omitempty"`
	Environment    string           `json:"environment,omitempty"`
	AgentSessionID string           `json:"agentSessionId,omitempty"`
	Project        grouping.Project `json:"project"`
}

func viewOf(s am.Session, p grouping.Project) SessionView {
	return SessionView{
		ID:             s.ID,
		Name:           s.Name,
		Tool:           s.Tool,
		Group:          s.Group,
		Directory:      s.Directory,
		Status:         s.Status,
		Running:        s.Running,
		Archived:       s.Archived,
		Branch:         s.Branch,
		Environment:    s.Environment,
		AgentSessionID: s.AgentSessionID,
		Project:        p,
	}
}
