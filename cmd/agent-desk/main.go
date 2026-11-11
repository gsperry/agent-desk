// Command agent-desk serves a local GUI for agent-manager sessions.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"html"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/gsperry/agent-desk/internal/am"
	"github.com/gsperry/agent-desk/internal/grouping"
	"github.com/gsperry/agent-desk/internal/hub"
	"github.com/gsperry/agent-desk/internal/locate"
	"github.com/gsperry/agent-desk/internal/manage"
	"github.com/gsperry/agent-desk/internal/open"
	"github.com/gsperry/agent-desk/internal/overview"
	"github.com/gsperry/agent-desk/internal/pane"
	procrun "github.com/gsperry/agent-desk/internal/run"
	"github.com/gsperry/agent-desk/internal/search"
	"github.com/gsperry/agent-desk/internal/send"
	"github.com/gsperry/agent-desk/internal/server"
	"github.com/gsperry/agent-desk/web"
)

var version = "dev"

func main() {
	port := flag.Int("port", 7317, "port on 127.0.0.1; a busy port falls back to a free one")
	noOpen := flag.Bool("no-open", false, "do not open the browser")
	devOrigins := flag.String("dev-origin", "", "comma-separated extra trusted origins (development only)")
	openBase := flag.String("open-base", "", "base URL to open instead of the server's own (development only)")
	fixtures := flag.String("fixtures", "", "serve sessions from a fixture directory instead of agent-manager (testing only)")
	showVersion := flag.Bool("version", false, "print the version")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	err := run(options{
		port:       *port,
		open:       !*noOpen,
		devOrigins: splitList(*devOrigins),
		openBase:   *openBase,
		fixtures:   *fixtures,
	})
	if err != nil {
		log.Fatal(err)
	}
}

type options struct {
	port       int
	open       bool
	devOrigins []string
	openBase   string
	fixtures   string
}

func run(o options) error {
	token, err := sessionToken()
	if err != nil {
		return err
	}
	poller, client, err := newPoller(o.fixtures)
	if err != nil {
		return err
	}
	transcripts, err := newTranscripts(o.fixtures, poller, client)
	if err != nil {
		return err
	}
	poller.Activity = transcriptActivity(transcripts.Locator)
	searcher := &search.Searcher{Sessions: poller, Locator: transcripts.Locator}
	l, err := listen(o.port)
	if err != nil {
		return err
	}
	addr := l.Addr().String()
	s, err := server.New(server.Config{
		Addr:        addr,
		Token:       token,
		Version:     version,
		DevOrigins:  o.devOrigins,
		Assets:      web.Assets(),
		Sessions:    poller,
		Manager:     client,
		Transcripts: transcripts,
		Sender:      &send.Sender{Sessions: poller, Pane: client},
		Panes:       &pane.Answerer{Sessions: poller, Reader: client, Keys: keys(o.fixtures)},
		Opener:      opener(o.fixtures, poller),
		Searcher:    searcher,
		Lifecycle:   &manage.Manager{Sessions: poller, Backend: client},
		Overview:    &overview.Overview{Sessions: poller, Locator: transcripts.Locator},
	})
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}

	base := "http://" + addr
	if o.openBase != "" {
		base = strings.TrimRight(o.openBase, "/")
	}
	url := base + "/#t=" + token
	fmt.Printf("Agent Desk %s listening on http://%s\nOpen: %s\n", version, addr, url)
	if o.open {
		openBrowser(url)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go poller.Run(ctx)
	go transcripts.Run(ctx)
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(l) }()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return srv.Shutdown(shutdown)
}

// backend is everything Agent Desk asks agent-manager to do, from the CLI or
// from the fixture.
type backend interface {
	am.Client
	manage.Backend
}

func newPoller(fixtures string) (*hub.Poller, backend, error) {
	home, _ := os.UserHomeDir()
	p := &hub.Poller{Resolver: &grouping.Resolver{Runner: procrun.Exec{Timeout: 3 * time.Second}, Home: home}}
	if fixtures != "" {
		f := am.Fixture{Path: filepath.Join(fixtures, "snapshot.json")}
		p.Client = f
		return p, f, nil
	}
	hooks, err := am.HooksDir()
	if err != nil {
		return nil, nil, err
	}
	c := am.NewCLI()
	p.Client, p.HooksDir = c, hooks
	return p, c, nil
}

// transcriptActivity is the transcript's modification time, for Claude
// sessions whose transcript exists.
func transcriptActivity(l *locate.Locator) func(context.Context, am.Session) (time.Time, bool) {
	return func(ctx context.Context, s am.Session) (time.Time, bool) {
		if s.Tool != "claude" || s.AgentSessionID == "" {
			return time.Time{}, false
		}
		path, err := l.Find(ctx, s.AgentSessionID, s.Environment)
		if err != nil {
			return time.Time{}, false
		}
		info, err := os.Stat(path)
		if err != nil {
			return time.Time{}, false
		}
		return info.ModTime(), true
	}
}

// newTranscripts reads Claude's transcripts, or <fixtures>/claude in
// fixture mode.
func newTranscripts(fixtures string, sessions *hub.Poller, client am.Client) (*hub.Transcripts, error) {
	root := filepath.Join(fixtures, "claude")
	if fixtures == "" {
		var err error
		if root, err = locate.DefaultRoot(); err != nil {
			return nil, err
		}
	}
	return &hub.Transcripts{
		Locator:  &locate.Locator{DefaultRoot: root, Profile: client.ProfileVar},
		Sessions: sessions,
	}, nil
}

// keys presses dialog keys through agent-manager's tmux server, or records
// them in <fixtures>/keys.log in fixture mode.
func keys(fixtures string) pane.Keys {
	if fixtures != "" {
		return pane.FileKeys{Path: filepath.Join(fixtures, "keys.log")}
	}
	return pane.TmuxKeys{Runner: procrun.Exec{Timeout: 5 * time.Second}}
}

// opener runs VS Code's code command, or records it in <fixtures>/open.log
// in fixture mode.
func opener(fixtures string, sessions *hub.Poller) *open.Opener {
	if fixtures != "" {
		return &open.Opener{Sessions: sessions, Runner: open.LogRunner{Path: filepath.Join(fixtures, "open.log")}, Editor: "code"}
	}
	return &open.Opener{Sessions: sessions, Runner: procrun.Exec{Timeout: 10 * time.Second}}
}

// sessionToken reads AGENT_DESK_TOKEN so `make dev` keeps one token across
// backend restarts. Otherwise each start gets a fresh random token.
func sessionToken() (string, error) {
	if t := os.Getenv("AGENT_DESK_TOKEN"); t != "" {
		// Child processes must not inherit it.
		if err := os.Unsetenv("AGENT_DESK_TOKEN"); err != nil {
			return "", err
		}
		if len(t) < 32 {
			return "", errors.New("AGENT_DESK_TOKEN must be at least 32 characters")
		}
		return t, nil
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func listen(port int) (net.Listener, error) {
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err == nil {
		return l, nil
	}
	log.Printf("port %d unavailable (%v), using a free port", port, err)
	return net.Listen("tcp", "127.0.0.1:0")
}

// openBrowser opens the app through a private launcher file rather than
// the URL itself, so the token never appears in a process's arguments,
// which other accounts on the machine can read.
func openBrowser(url string) {
	dir, err := os.UserCacheDir()
	if err != nil {
		log.Printf("could not open a browser: %v", err)
		return
	}
	path, err := writeLauncher(filepath.Join(dir, "agent-desk"), url)
	if err != nil {
		log.Printf("could not open a browser: %v", err)
		return
	}
	// The browser reads the file within moments; it is not needed after.
	time.AfterFunc(30*time.Second, func() { _ = os.Remove(path) })
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("could not open a browser: %v", err)
		return
	}
	go func() { _ = cmd.Wait() }()
}

// writeLauncher writes an owner-only HTML file that redirects to url, in an
// owner-only directory, and returns its path.
func writeLauncher(dir, url string) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, "open-*.html")
	if err != nil {
		return "", err
	}
	defer f.Close()
	u := html.EscapeString(url)
	page := `<!doctype html><meta charset="utf-8"><meta http-equiv="refresh" content="0;url=` + u + `"><title>Agent Desk</title><p><a href="` + u + `">Open Agent Desk</a></p>`
	if _, err := f.WriteString(page); err != nil {
		return "", err
	}
	return f.Name(), nil
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
