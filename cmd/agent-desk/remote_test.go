package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gsperry/agent-desk/internal/remote"
)

func TestRemoteListenerSocket(t *testing.T) {
	// A short directory: Unix socket paths are limited to about 100 bytes.
	dir, err := os.MkdirTemp("/tmp", "ad-remote-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	store := &remote.Store{Path: filepath.Join(dir, "conf", "remote.json")}
	r := newRemoteListener(store, "")
	r.handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	var changed []remote.Settings
	r.onChange = func(s remote.Settings) { changed = append(changed, s) }

	// A socket left by an earlier run is replaced.
	if err := os.MkdirAll(filepath.Dir(r.socket), 0o755); err != nil {
		t.Fatal(err)
	}
	stale, err := net.Listen("unix", r.socket)
	if err != nil {
		t.Fatal(err)
	}
	_ = stale.(*net.UnixListener).Close()
	if _, err := os.Lstat(r.socket); os.IsNotExist(err) {
		// Some platforms unlink on close; make sure one is there.
		l, _ := net.Listen("unix", r.socket)
		l.(*net.UnixListener).SetUnlinkOnClose(false)
		_ = l.Close()
	}

	on := remote.Settings{Enabled: true, Level: remote.LevelAnswer, Host: "desk.ts.net", TailscaleLogins: []string{"a@b.c"}}
	if _, err := r.Set(on); err != nil {
		t.Fatal(err)
	}
	st := r.Status()
	if !st.Listening || st.Address != "unix:"+r.socket || st.Error != "" {
		t.Fatalf("status = %+v", st)
	}
	info, err := os.Stat(r.socket)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("socket mode %v err %v", info.Mode(), err)
	}
	dinfo, _ := os.Stat(filepath.Dir(r.socket))
	if dinfo.Mode().Perm() != 0o700 {
		t.Fatalf("socket directory mode %v", dinfo.Mode())
	}
	client := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", r.socket)
	}}, Timeout: 2 * time.Second}
	res, err := client.Get("http://desk.ts.net/")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()

	// Turning it off stops the listener.
	off := on
	off.Enabled = false
	if _, err := r.Set(off); err != nil {
		t.Fatal(err)
	}
	if st := r.Status(); st.Listening {
		t.Fatalf("still listening: %+v", st)
	}
	if _, err := client.Get("http://desk.ts.net/"); err == nil {
		t.Fatal("served after remote access was turned off")
	}
	if len(changed) != 2 || changed[1].Enabled {
		t.Fatalf("onChange calls = %+v", changed)
	}
	r.stop()
}

func TestRemoteListenerRefusesToReplaceAFile(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "ad-remote-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	store := &remote.Store{Path: filepath.Join(dir, "remote.json")}
	r := newRemoteListener(store, "")
	r.handler = http.NotFoundHandler()
	if err := os.WriteFile(r.socket, []byte("not a socket"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Set(remote.Settings{Enabled: true, Host: "desk.ts.net", TailscaleLogins: []string{"a@b.c"}}); err != nil {
		t.Fatal(err)
	}
	if st := r.Status(); st.Listening || st.Error == "" {
		t.Fatalf("status = %+v, want an error", st)
	}
	if data, _ := os.ReadFile(r.socket); string(data) != "not a socket" {
		t.Fatal("a regular file was replaced")
	}
}

func TestRemoteListenerLeavesALiveSocket(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "ad-remote-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	store := &remote.Store{Path: filepath.Join(dir, "remote.json")}
	first := newRemoteListener(store, "")
	first.handler = http.NotFoundHandler()
	on := remote.Settings{Enabled: true, Host: "desk.ts.net", TailscaleLogins: []string{"a@b.c"}}
	if _, err := first.Set(on); err != nil || !first.Status().Listening {
		t.Fatalf("first: %v %+v", err, first.Status())
	}
	defer first.stop()
	second := newRemoteListener(&remote.Store{Path: store.Path}, "")
	second.handler = http.NotFoundHandler()
	if err := second.apply(on); err == nil || second.Status().Listening {
		t.Fatalf("second took over a live socket: %v %+v", err, second.Status())
	}
	if c, err := net.Dial("unix", first.socket); err != nil {
		t.Fatalf("first listener lost its socket: %v", err)
	} else {
		_ = c.Close()
	}
}

// When the Agent Desk holding the socket stops, the other one takes over
// remote access on its own.
func TestRemoteListenerTakesOverWhenTheOtherStops(t *testing.T) {
	old := remoteRetry
	remoteRetry = 50 * time.Millisecond
	defer func() { remoteRetry = old }()
	dir, err := os.MkdirTemp("/tmp", "ad-remote-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	store := &remote.Store{Path: filepath.Join(dir, "remote.json")}
	on := remote.Settings{Enabled: true, Host: "desk.ts.net", TailscaleLogins: []string{"a@b.c"}}
	first := newRemoteListener(store, "")
	first.handler = http.NotFoundHandler()
	if _, err := first.Set(on); err != nil || !first.Status().Listening {
		t.Fatalf("first: %v %+v", err, first.Status())
	}
	second := newRemoteListener(&remote.Store{Path: store.Path}, "")
	second.handler = http.NotFoundHandler()
	defer second.stop()
	if err := second.apply(on); err == nil {
		t.Fatal("second took the socket while the first held it")
	}
	first.stop()
	deadline := time.Now().Add(3 * time.Second)
	for !second.Status().Listening {
		if time.Now().After(deadline) {
			t.Fatalf("second never took over: %+v", second.Status())
		}
		time.Sleep(20 * time.Millisecond)
	}
	// Turned off, it stops trying.
	if _, err := second.Set(remote.Settings{}); err != nil || second.Status().Listening {
		t.Fatalf("off: %v %+v", err, second.Status())
	}
}
