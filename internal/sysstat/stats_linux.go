package sysstat

import (
	"fmt"
	"os"
	"runtime"

	"golang.org/x/sys/unix"
)

func read(paths []string) Stats {
	st := Stats{Supported: true, Disks: disks(paths, statfs)}
	st.CPU.Cores = runtime.NumCPU()
	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		st.CPU.Load = parseProcLoadavg(b)
	}
	if b, err := os.ReadFile("/proc/meminfo"); err == nil {
		st.Memory, st.Swap = parseMeminfo(b)
	}
	return st
}

// statfs names a filesystem by the path asked about; Linux's statfs does
// not report the mount point.
func statfs(path string) (string, Disk, error) {
	var s unix.Statfs_t
	if err := unix.Statfs(path, &s); err != nil {
		return "", Disk{}, err
	}
	return fmt.Sprint(s.Fsid), Disk{
		Mount: path,
		Total: s.Blocks * uint64(s.Bsize),
		Free:  s.Bavail * uint64(s.Bsize),
	}, nil
}
