package sysstat

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"runtime"
	"testing"
	"time"
)

func TestParseLoadavg(t *testing.T) {
	raw := make([]byte, 24)
	binary.LittleEndian.PutUint32(raw[0:], 4096)
	binary.LittleEndian.PutUint32(raw[4:], 2048)
	binary.LittleEndian.PutUint32(raw[8:], 1024)
	binary.LittleEndian.PutUint64(raw[16:], 2048)
	if got := parseLoadavg(raw); got != [3]float64{2, 1, 0.5} {
		t.Errorf("load = %v", got)
	}
	if got := parseLoadavg(raw[:10]); got != [3]float64{} {
		t.Errorf("short = %v", got)
	}
	if got := parseLoadavg(make([]byte, 24)); got != [3]float64{} {
		t.Errorf("zero scale = %v", got)
	}
}

func TestPressure(t *testing.T) {
	for level, want := range map[uint32]string{1: "normal", 2: "warn", 4: "critical", 0: "", 3: ""} {
		if got := pressure(level); got != want {
			t.Errorf("pressure(%d) = %q, want %q", level, got, want)
		}
	}
}

func TestParseProcLoadavg(t *testing.T) {
	if got := parseProcLoadavg([]byte("0.52 1.25 2.00 3/812 12345\n")); got != [3]float64{0.52, 1.25, 2} {
		t.Errorf("load = %v", got)
	}
	if got := parseProcLoadavg([]byte("junk")); got != [3]float64{} {
		t.Errorf("junk = %v", got)
	}
}

func TestParseMeminfo(t *testing.T) {
	in := "MemTotal:       16000000 kB\nMemFree:  1000 kB\nMemAvailable:    4000000 kB\nSwapTotal:       2000000 kB\nSwapFree:        1500000 kB\nbad line\nHugePages_Total: 0\n"
	m, s := parseMeminfo([]byte(in))
	if m.Total != 16000000*1024 || m.Available != 4000000*1024 || m.Pressure != "" {
		t.Errorf("memory = %+v", m)
	}
	if s.Total != 2000000*1024 || s.Used != 500000*1024 {
		t.Errorf("swap = %+v", s)
	}
	// SwapFree above SwapTotal never underflows.
	_, s = parseMeminfo([]byte("SwapTotal: 10 kB\nSwapFree: 20 kB\n"))
	if s.Used != 0 {
		t.Errorf("swap used = %d", s.Used)
	}
}

func TestDisks(t *testing.T) {
	fs := map[string]struct {
		id string
		d  Disk
	}{
		"/home/dev":     {"a", Disk{Mount: "/", Total: 100, Free: 40}},
		"/home/dev/src": {"a", Disk{Mount: "/", Total: 100, Free: 40}},
		"/mnt/data/x":   {"b", Disk{Mount: "/mnt/data", Total: 500, Free: 5}},
		"/proc/empty":   {"c", Disk{Mount: "/proc"}},
	}
	statfs := func(p string) (string, Disk, error) {
		f, ok := fs[p]
		if !ok {
			return "", Disk{}, errors.New("gone")
		}
		return f.id, f.d, nil
	}
	got := disks([]string{"/home/dev", "", "/gone", "/home/dev/src", "/proc/empty", "/mnt/data/x"}, statfs)
	if len(got) != 2 || got[0].Mount != "/" || got[1].Mount != "/mnt/data" {
		t.Errorf("disks = %+v", got)
	}
}

func TestSamplerReadsThisMachine(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("no reader for", runtime.GOOS)
	}
	home, _ := os.UserHomeDir()
	s := &Sampler{Paths: func() []string { return []string{home, os.TempDir()} }, Every: time.Hour}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.Run(ctx)
		close(done)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for s.Latest().At.IsZero() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
	st := s.Latest()
	if !st.Supported || st.CPU.Cores < 1 || st.Memory.Total == 0 || st.Memory.Available > st.Memory.Total || len(st.Disks) == 0 {
		t.Fatalf("stats = %+v", st)
	}
	t.Logf("%+v", st)
	for _, d := range st.Disks {
		if d.Free > d.Total || d.Mount == "" {
			t.Errorf("disk = %+v", d)
		}
	}
}
