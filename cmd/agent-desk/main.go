// Command agent-desk serves a local GUI for agent-manager sessions.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
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
	procrun "github.com/gsperry/agent-desk/internal/run"
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

func newPoller(fixtures string) (*hub.Poller, am.Client, error) {
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

// sessionToken reads AGENT_DESK_TOKEN so `make dev` keeps one token across
// backend restarts. Otherwise each start gets a fresh random token.
func sessionToken() (string, error) {
	if t := os.Getenv("AGENT_DESK_TOKEN"); t != "" {
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

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("could not open a browser: %v", err)
		return
	}
	go func() { _ = cmd.Wait() }()
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
