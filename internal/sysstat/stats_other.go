//go:build !darwin && !linux

package sysstat

func read([]string) Stats { return Stats{Disks: []Disk{}} }
