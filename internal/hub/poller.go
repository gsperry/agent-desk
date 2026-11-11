package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/gsperry/agent-desk/internal/am"
	"github.com/gsperry/agent-desk/internal/grouping"
)

// Poller reads agent-manager's snapshot on an interval, and early when a
// hook status file changes, and publishes the State when it differs.
type Poller struct {
	Client   am.Client
	Resolver *grouping.Resolver
	Interval time.Duration
	// HooksDir is watched for *.status writes. Empty disables the watcher.
	HooksDir string

	mu      sync.Mutex
	state   State
	encoded []byte
	subs    map[chan State]struct{}
	trigger chan struct{}
	once    sync.Once
}

func (p *Poller) init() {
	p.once.Do(func() {
		p.trigger = make(chan struct{}, 1)
		p.subs = map[chan State]struct{}{}
		p.state = State{Sessions: []SessionView{}}
	})
}

// Trigger asks for a poll now. Calls while one is pending coalesce.
func (p *Poller) Trigger() {
	p.init()
	select {
	case p.trigger <- struct{}{}:
	default:
	}
}

func (p *Poller) State() State {
	p.init()
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

// Subscribe returns a channel that always holds the newest State. A slow
// reader skips intermediate states rather than blocking the poller.
func (p *Poller) Subscribe() (<-chan State, func()) {
	p.init()
	ch := make(chan State, 1)
	p.mu.Lock()
	p.subs[ch] = struct{}{}
	p.mu.Unlock()
	return ch, func() {
		p.mu.Lock()
		delete(p.subs, ch)
		p.mu.Unlock()
	}
}

func (p *Poller) Run(ctx context.Context) {
	p.init()
	interval := p.Interval
	if interval == 0 {
		interval = time.Second
	}
	var watcher *fsnotify.Watcher
	defer func() {
		if watcher != nil {
			_ = watcher.Close()
		}
	}()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if watcher == nil && p.HooksDir != "" {
			watcher = p.watch(ctx)
		}
		p.Poll(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-p.trigger:
		}
	}
}

// watch starts the hook watcher, or returns nil to retry on the next poll
// (the directory appears with the first agent-manager session).
func (p *Poller) watch(ctx context.Context) *fsnotify.Watcher {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil
	}
	if err := w.Add(p.HooksDir); err != nil {
		_ = w.Close()
		return nil
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				if strings.HasSuffix(ev.Name, ".status") && ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove) != 0 {
					p.Trigger()
				}
			case _, ok := <-w.Errors:
				if !ok {
					return
				}
			}
		}
	}()
	return w
}

// Poll reads one snapshot and publishes the result if it changed.
func (p *Poller) Poll(ctx context.Context) {
	p.init()
	p.publish(p.build(ctx))
}

func (p *Poller) build(ctx context.Context) State {
	snap, err := p.Client.Snapshot(ctx)
	if err != nil {
		kind := ErrFailed
		if errors.Is(err, am.ErrNotInstalled) {
			kind = ErrNotInstalled
		}
		// Keep the last good list so a transient failure does not blank the
		// sidebar; the error banner says it may be stale.
		prev := p.State()
		prev.Error = &StateError{Kind: kind, Message: err.Error()}
		return prev
	}
	st := State{ManagerAwake: snap.ManagerAwake, Sessions: make([]SessionView, 0, len(snap.Sessions))}
	for _, s := range snap.Sessions {
		if s.Self {
			st.InsideSession = true
		}
		st.Sessions = append(st.Sessions, viewOf(s, p.Resolver.Resolve(ctx, s)))
	}
	return st
}

func (p *Poller) publish(st State) {
	encoded, err := json.Marshal(st)
	if err != nil {
		log.Printf("encode state: %v", err)
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if bytes.Equal(encoded, p.encoded) {
		return
	}
	p.state, p.encoded = st, encoded
	for ch := range p.subs {
		select {
		case <-ch:
		default:
		}
		ch <- st
	}
}
