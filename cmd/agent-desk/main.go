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
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/gsperry/agent-desk/internal/server"
	"github.com/gsperry/agent-desk/web"
)

var version = "dev"

func main() {
	port := flag.Int("port", 7317, "port on 127.0.0.1; a busy port falls back to a free one")
	noOpen := flag.Bool("no-open", false, "do not open the browser")
	devOrigins := flag.String("dev-origin", "", "comma-separated extra trusted origins (development only)")
	openBase := flag.String("open-base", "", "base URL to open instead of the server's own (development only)")
	showVersion := flag.Bool("version", false, "print the version")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	if err := run(*port, !*noOpen, splitList(*devOrigins), *openBase); err != nil {
		log.Fatal(err)
	}
}

func run(port int, open bool, devOrigins []string, openBase string) error {
	token, err := sessionToken()
	if err != nil {
		return err
	}
	l, err := listen(port)
	if err != nil {
		return err
	}
	addr := l.Addr().String()
	s, err := server.New(server.Config{
		Addr:       addr,
		Token:      token,
		Version:    version,
		DevOrigins: devOrigins,
		Assets:     web.Assets(),
	})
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}

	base := "http://" + addr
	if openBase != "" {
		base = strings.TrimRight(openBase, "/")
	}
	url := base + "/#t=" + token
	fmt.Printf("Agent Desk %s listening on http://%s\nOpen: %s\n", version, addr, url)
	if open {
		openBrowser(url)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
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
