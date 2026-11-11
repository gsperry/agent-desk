// Package open opens a file from a transcript in VS Code.
package open

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/gsperry/agent-desk/internal/hub"
	"github.com/gsperry/agent-desk/internal/run"
	"github.com/gsperry/agent-desk/internal/send"
)

type Sessions interface {
	State() hub.State
}

type Request struct {
	SessionID string `json:"sessionId"`
	Path      string `json:"path"`
	Line      int    `json:"line"`
}

const (
	reasonNotFound   send.Reason = "not_found"
	reasonNoEditor   send.Reason = "no_editor"
	reasonBadRequest send.Reason = "bad_request"
)

// macOSCode is where VS Code keeps its CLI when "Install 'code' command in
// PATH" was never run.
const macOSCode = "/Applications/Visual Studio Code.app/Contents/Resources/app/bin/code"

type Opener struct {
	Sessions Sessions
	Runner   run.Runner
	// Editor is the command to run; empty finds VS Code's code.
	Editor string
}

// Open runs `code -g path[:line]` for a path that exists. A relative path
// resolves against the session's working directory.
func (o *Opener) Open(ctx context.Context, req Request) error {
	path, err := o.resolve(req)
	if err != nil {
		return err
	}
	editor := o.Editor
	if editor == "" {
		if editor, err = findCode(); err != nil {
			return err
		}
	}
	target := path
	if req.Line > 0 {
		target += ":" + strconv.Itoa(req.Line)
	}
	_, err = o.Runner.Run(ctx, editor, "-g", target)
	return err
}

func (o *Opener) resolve(req Request) (string, error) {
	p := strings.TrimSpace(strings.TrimPrefix(req.Path, "file://"))
	if p == "" || req.Line < 0 || strings.ContainsRune(p, 0) {
		return "", &send.Refused{Reason: reasonBadRequest, Message: "No file path to open."}
	}
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, p[2:])
		}
	}
	if !filepath.IsAbs(p) {
		dir := ""
		for _, s := range o.Sessions.State().Sessions {
			if s.ID == req.SessionID {
				dir = s.Directory
			}
		}
		if dir == "" {
			return "", &send.Refused{Reason: reasonNotFound, Message: fmt.Sprintf("Cannot resolve %s without the session's directory.", p)}
		}
		p = filepath.Join(dir, p)
	}
	p = filepath.Clean(p)
	if _, err := os.Stat(p); err != nil {
		return "", &send.Refused{Reason: reasonNotFound, Message: fmt.Sprintf("%s does not exist.", p)}
	}
	return p, nil
}

func findCode() (string, error) {
	if p, err := exec.LookPath("code"); err == nil {
		return p, nil
	}
	if runtime.GOOS == "darwin" {
		if _, err := os.Stat(macOSCode); err == nil {
			return macOSCode, nil
		}
	}
	return "", &send.Refused{Reason: reasonNoEditor, Message: "VS Code's code command was not found. In VS Code, run \"Shell Command: Install 'code' command in PATH\"."}
}

// LogRunner records commands in a file instead of running them, for
// fixture mode.
type LogRunner struct {
	Path string
}

func (l LogRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	f, err := os.OpenFile(l.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	_, err = fmt.Fprintln(f, strings.Join(append([]string{filepath.Base(name)}, args...), " "))
	return nil, err
}
