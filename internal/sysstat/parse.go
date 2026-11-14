package sysstat

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"strconv"
	"strings"
)

// parseLoadavg decodes struct loadavg: three fixed-point loads, then the
// scale as a long at offset 16.
func parseLoadavg(raw []byte) [3]float64 {
	var out [3]float64
	if len(raw) < 24 {
		return out
	}
	scale := float64(binary.LittleEndian.Uint64(raw[16:]))
	if scale == 0 {
		return out
	}
	for i := range out {
		out[i] = float64(binary.LittleEndian.Uint32(raw[i*4:])) / scale
	}
	return out
}

// pressure names macOS's memory pressure levels.
func pressure(level uint32) string {
	switch level {
	case 1:
		return "normal"
	case 2:
		return "warn"
	case 4:
		return "critical"
	}
	return ""
}

func parseProcLoadavg(b []byte) [3]float64 {
	var out [3]float64
	f := strings.Fields(string(b))
	for i := 0; i < 3 && i < len(f); i++ {
		out[i], _ = strconv.ParseFloat(f[i], 64)
	}
	return out
}

// parseMeminfo reads MemTotal, MemAvailable, SwapTotal and SwapFree, in kB.
func parseMeminfo(b []byte) (Memory, Swap) {
	kb := map[string]uint64{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		name, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		f := strings.Fields(rest)
		if len(f) == 0 {
			continue
		}
		if n, err := strconv.ParseUint(f[0], 10, 64); err == nil {
			kb[name] = n * 1024
		}
	}
	m := Memory{Total: kb["MemTotal"], Available: min(kb["MemAvailable"], kb["MemTotal"])}
	s := Swap{Total: kb["SwapTotal"], Used: kb["SwapTotal"] - min(kb["SwapFree"], kb["SwapTotal"])}
	return m, s
}
