// Package sysstat reads the computer's load, memory, swap and free disk
// space, so many agents running builds at once can be seen running the
// machine short. It reads them from the kernel (sysctl and statfs on macOS,
// /proc and statfs on Linux) and runs no commands.
package sysstat

import (
	"context"
	"sync"
	"time"
)

type Stats struct {
	CPU    CPU    `json:"cpu"`
	Memory Memory `json:"memory"`
	Swap   Swap   `json:"swap"`
	Disks  []Disk `json:"disks"`
	// Supported is false on a system this package cannot read.
	Supported bool      `json:"supported"`
	At        time.Time `json:"at"`
}

// CPU is the load average over 1, 5 and 15 minutes and the core count.
type CPU struct {
	Load  [3]float64 `json:"load"`
	Cores int        `json:"cores"`
}

// Memory is in bytes. Available is what can be had without swapping: free
// memory plus caches the system drops on demand. Pressure is the system's
// own reading where it has one: normal, warn or critical.
type Memory struct {
	Total     uint64 `json:"total"`
	Available uint64 `json:"available"`
	Pressure  string `json:"pressure,omitempty"`
}

type Swap struct {
	Total uint64 `json:"total"`
	Used  uint64 `json:"used"`
}

// Disk is one filesystem, named by where it is mounted.
type Disk struct {
	Mount string `json:"mount"`
	Total uint64 `json:"total"`
	Free  uint64 `json:"free"`
}

// Sampler reads the stats on a timer and keeps the latest, so a request
// never waits on the kernel or on a slow filesystem.
type Sampler struct {
	// Paths returns the directories whose filesystems are reported: the
	// home directory and every session's directory.
	Paths func() []string
	// Every is the refresh interval; 5 s when zero.
	Every time.Duration

	mu     sync.Mutex
	latest Stats
}

func (s *Sampler) Latest() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.latest
}

func (s *Sampler) Run(ctx context.Context) {
	every := s.Every
	if every == 0 {
		every = 5 * time.Second
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		s.sample()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (s *Sampler) sample() {
	var paths []string
	if s.Paths != nil {
		paths = s.Paths()
	}
	st := read(paths)
	st.At = time.Now()
	s.mu.Lock()
	s.latest = st
	s.mu.Unlock()
}

// disks reports the filesystems holding paths, each once, in the order
// first seen. Paths that cannot be read are skipped.
func disks(paths []string, statfs func(string) (id string, d Disk, err error)) []Disk {
	seen := map[string]bool{}
	out := []Disk{}
	for _, p := range paths {
		if p == "" {
			continue
		}
		id, d, err := statfs(p)
		if err != nil || seen[id] || d.Total == 0 {
			continue
		}
		seen[id] = true
		out = append(out, d)
	}
	return out
}
