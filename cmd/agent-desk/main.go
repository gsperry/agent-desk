// Command agent-desk serves a local GUI for agent-manager sessions.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
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
	"sync"
	"syscall"
	"time"

	"github.com/gsperry/agent-desk/internal/am"
	"github.com/gsperry/agent-desk/internal/changes"
	"github.com/gsperry/agent-desk/internal/conflicts"
	"github.com/gsperry/agent-desk/internal/grouping"
	"github.com/gsperry/agent-desk/internal/history"
	"github.com/gsperry/agent-desk/internal/hub"
	"github.com/gsperry/agent-desk/internal/locate"
	"github.com/gsperry/agent-desk/internal/manage"
	"github.com/gsperry/agent-desk/internal/open"
	"github.com/gsperry/agent-desk/internal/overview"
	"github.com/gsperry/agent-desk/internal/pane"
	"github.com/gsperry/agent-desk/internal/paste"
	"github.com/gsperry/agent-desk/internal/remote"
	procrun "github.com/gsperry/agent-desk/internal/run"
	"github.com/gsperry/agent-desk/internal/search"
	"github.com/gsperry/agent-desk/internal/send"
	"github.com/gsperry/agent-desk/internal/server"
	"github.com/gsperry/agent-desk/internal/summary"
	"github.com/gsperry/agent-desk/internal/usage"
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
	pastes, err := pasteStore(o.fixtures)
	if err != nil {
		return err
	}
	remoteStore, err := remoteSettingsStore(o.fixtures)
	if err != nil {
		return err
	}
	remoteAccess := newRemoteListener(remoteStore, o.fixtures)
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
		Panes:       &pane.Answerer{Sessions: poller, Reader: client, Keys: keys(o.fixtures), Typer: client},
		Opener:      opener(o.fixtures, poller),
		Searcher:    searcher,
		Lifecycle:   &manage.Manager{Sessions: poller, Backend: client},
		Overview:    &overview.Overview{Sessions: poller, Locator: transcripts.Locator},
		Conflicts:   &conflicts.Finder{Sessions: poller, Locator: transcripts.Locator},
		Pastes:      pastes,
		History:     &history.Index{Root: transcripts.Locator.DefaultRoot},
		Usage:       &usage.Scanner{Root: transcripts.Locator.DefaultRoot},
		Summaries:   &summary.Summarizer{Runner: summaryRunner(o.fixtures), Transcripts: transcripts},
		Remote:      remoteAccess,
		// Browser tests reach the remote listener over plain HTTP.
		RemoteHTTPOrigin: o.fixtures != "",
		Changes: &changes.Service{Sessions: poller, Reader: changes.Reader{Runner: procrun.Exec{
			Timeout: 15 * time.Second,
			Env:     changes.Env(procrun.Environ()),
		}}},
	})
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}
	remoteAccess.handler = s.RemoteHandler()
	remoteAccess.onChange = s.DropRemote
	if err := remoteAccess.apply(remoteStore.Get()); err != nil {
		log.Printf("remote access: %v", err)
	}
	defer remoteAccess.stop()

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
	go cleanPastes(ctx, pastes)
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

// remoteSettingsStore keeps remote access settings in the user config
// directory, or the fixture directory in fixture mode.
func remoteSettingsStore(fixtures string) (*remote.Store, error) {
	if fixtures != "" {
		return &remote.Store{Path: filepath.Join(fixtures, "remote.json")}, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return &remote.Store{Path: filepath.Join(dir, "agent-desk", "remote.json")}, nil
}

// newRemoteListener serves remote clients on a Unix socket in an
// owner-only directory, so no other account on the machine can connect;
// tailscale serve (run by root) forwards to it. Fixture mode listens on a
// 127.0.0.1 TCP port instead, so browser tests can reach it.
func newRemoteListener(store *remote.Store, fixtures string) *remoteListener {
	r := &remoteListener{store: store, tcp: fixtures != ""}
	if !r.tcp {
		r.socket = filepath.Join(filepath.Dir(store.Path), "remote.sock")
	}
	return r
}

type remoteListener struct {
	store   *remote.Store
	handler http.Handler
	// onChange closes remote connections the new settings no longer allow.
	onChange func(remote.Settings)
	socket   string
	tcp      bool

	mu     sync.Mutex
	srv    *http.Server
	addr   string
	status server.RemoteStatus
}

func (r *remoteListener) Get() remote.Settings { return r.store.Get() }

// Status reports the listener and, when it is not running, the address it
// would use, so the settings screen can show the tailscale serve command.
func (r *remoteListener) Status() server.RemoteStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.status
	if st.Address == "" {
		st.Address = r.address(r.store.Get())
	}
	return st
}

// Set saves the settings, then starts, moves or stops the listener to
// match. A listener that fails to start is reported in Status.
func (r *remoteListener) Set(s remote.Settings) (remote.Settings, error) {
	saved, err := r.store.Set(s)
	if err != nil {
		return remote.Settings{}, err
	}
	if err := r.apply(saved); err != nil {
		log.Printf("remote access: %v", err)
	}
	if r.onChange != nil {
		r.onChange(saved)
	}
	return saved, nil
}

func (r *remoteListener) address(s remote.Settings) string {
	if r.tcp {
		return fmt.Sprintf("127.0.0.1:%d", s.Port)
	}
	return "unix:" + r.socket
}

func (r *remoteListener) apply(s remote.Settings) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	want := r.address(s)
	if r.srv != nil && (!s.Enabled || want != r.addr) {
		_ = r.srv.Close()
		r.srv = nil
		log.Printf("remote access stopped")
	}
	if !s.Enabled {
		r.status = server.RemoteStatus{}
		return nil
	}
	if r.srv != nil || r.handler == nil {
		return nil
	}
	l, err := r.listen(s)
	if err != nil {
		r.status = server.RemoteStatus{Address: want, Error: err.Error()}
		return err
	}
	r.srv = &http.Server{Handler: r.handler, ReadHeaderTimeout: 10 * time.Second}
	if !r.tcp {
		r.srv.ConnContext = func(ctx context.Context, _ net.Conn) context.Context { return server.OnSocket(ctx) }
	}
	r.addr = want
	r.status = server.RemoteStatus{Listening: true, Address: want}
	go func(srv *http.Server) { _ = srv.Serve(l) }(r.srv)
	log.Printf("remote access on %s for https://%s (level %s)", want, s.Host, s.Level)
	return nil
}

func (r *remoteListener) listen(s remote.Settings) (net.Listener, error) {
	if r.tcp {
		return net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", s.Port))
	}
	dir := filepath.Dir(r.socket)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, err
	}
	// A socket left by an earlier run is removed; a live one (another
	// Agent Desk) or anything else is not.
	if len(r.socket) > 100 {
		return nil, fmt.Errorf("the socket path %s is too long for a Unix socket", r.socket)
	}
	if info, err := os.Lstat(r.socket); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("%s exists and is not a socket", r.socket)
		}
		if c, err := net.DialTimeout("unix", r.socket, time.Second); err == nil {
			_ = c.Close()
			return nil, fmt.Errorf("another Agent Desk is already serving remote access at %s", r.socket)
		}
		if err := os.Remove(r.socket); err != nil {
			return nil, err
		}
	}
	l, err := net.Listen("unix", r.socket)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(r.socket, 0o600); err != nil {
		_ = l.Close()
		return nil, err
	}
	return l, nil
}

func (r *remoteListener) stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.srv != nil {
		_ = r.srv.Close()
	}
}

// summaryRunner runs the user's Claude Code CLI for catch-up summaries. In
// fixture mode it answers with a canned result, so tests never call a model.
func summaryRunner(fixtures string) summary.Runner {
	if fixtures != "" {
		return fixtureSummaries{}
	}
	return procrun.Exec{Timeout: 3 * time.Minute}
}

type fixtureSummaries struct{}

func (fixtureSummaries) RunInput(_ context.Context, _ string, input []byte, _ string, _ ...string) ([]byte, error) {
	turns := strings.Count(string(input), "\n## ")
	return json.Marshal(map[string]any{
		"type": "result", "is_error": false,
		"result":         fmt.Sprintf("- Fixture summary of %d sections.\n- No model was called.", turns),
		"total_cost_usd": 0.0012,
		"usage":          map[string]int{"input_tokens": 1500, "output_tokens": 30},
	})
}

// pasteStore keeps pasted images under the user cache directory, or the
// fixture directory in fixture mode.
func pasteStore(fixtures string) (paste.Store, error) {
	if fixtures != "" {
		// Agents read the path from anywhere, so it must be absolute.
		dir, err := filepath.Abs(filepath.Join(fixtures, "pastes"))
		return paste.Store{Dir: dir}, err
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return paste.Store{}, err
	}
	return paste.Store{Dir: filepath.Join(dir, "agent-desk", "pastes")}, nil
}

// cleanPastes removes week-old pasted images at start and once a day.
func cleanPastes(ctx context.Context, s paste.Store) {
	const age = 7 * 24 * time.Hour
	s.Cleanup(age, time.Now())
	t := time.NewTicker(24 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			s.Cleanup(age, now)
		}
	}
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
