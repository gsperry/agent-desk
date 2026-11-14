package sysstat

import (
	"encoding/binary"
	"fmt"
	"runtime"

	"golang.org/x/sys/unix"
)

func read(paths []string) Stats {
	st := Stats{Supported: true, Disks: disks(paths, statfs)}
	st.CPU.Cores = runtime.NumCPU()
	if raw, err := unix.SysctlRaw("vm.loadavg"); err == nil {
		st.CPU.Load = parseLoadavg(raw)
	}
	st.Memory = memory()
	if raw, err := unix.SysctlRaw("vm.swapusage"); err == nil && len(raw) >= 24 {
		st.Swap = Swap{Total: binary.LittleEndian.Uint64(raw[0:]), Used: binary.LittleEndian.Uint64(raw[16:])}
	}
	return st
}

// memory counts as available the free and speculative pages and the file
// cache (external and purgeable pages), which macOS drops before it
// compresses or swaps; Activity Monitor's "Memory Used" is the rest.
func memory() Memory {
	var m Memory
	m.Total, _ = unix.SysctlUint64("hw.memsize")
	page := uint64(unix.Getpagesize())
	var pages uint64
	for _, name := range []string{"vm.page_free_count", "vm.page_speculative_count", "vm.page_pageable_external_count", "vm.page_purgeable_count"} {
		if n, err := unix.SysctlUint32(name); err == nil {
			pages += uint64(n)
		}
	}
	m.Available = min(pages*page, m.Total)
	if level, err := unix.SysctlUint32("kern.memorystatus_vm_pressure_level"); err == nil {
		m.Pressure = pressure(level)
	}
	return m
}

func statfs(path string) (string, Disk, error) {
	var s unix.Statfs_t
	if err := unix.Statfs(path, &s); err != nil {
		return "", Disk{}, err
	}
	return fmt.Sprint(s.Fsid), Disk{
		Mount: unix.ByteSliceToString(s.Mntonname[:]),
		Total: s.Blocks * uint64(s.Bsize),
		Free:  s.Bavail * uint64(s.Bsize),
	}, nil
}
