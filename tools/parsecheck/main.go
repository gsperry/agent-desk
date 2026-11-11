// Command parsecheck runs the transcript parser over real transcripts and
// reports raw items by reason, so a new Claude Code record shape shows up
// before it shows up in the UI. It prints labels only, never content.
//
//	go run ./tools/parsecheck ~/.claude/projects/*/*.jsonl
package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"sort"
	"time"

	"github.com/gsperry/agent-desk/internal/transcript"
)

func main() {
	reasons := map[string]int{}
	var lines, items, raw int
	var slowest time.Duration
	var slowestFile string
	for _, path := range os.Args[1:] {
		start := time.Now()
		f, err := os.Open(path)
		if err != nil {
			log.Fatal(err)
		}
		p := transcript.NewParser()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 256<<20)
		for sc.Scan() {
			for _, it := range p.Line(sc.Bytes()) {
				if it.Kind == transcript.KindRaw {
					reasons[it.Label]++
				}
			}
		}
		if err := sc.Err(); err != nil {
			log.Printf("%s: %v", path, err)
		}
		f.Close()
		st := p.Stats()
		lines, items, raw = lines+st.Lines, items+st.Items, raw+st.Raw
		if d := time.Since(start); d > slowest {
			slowest, slowestFile = d, path
		}
	}
	fmt.Printf("files=%d lines=%d items=%d raw=%d slowest=%s (%s)\n", len(os.Args)-1, lines, items, raw, slowest.Round(time.Millisecond), slowestFile)
	keys := make([]string, 0, len(reasons))
	for k := range reasons {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return reasons[keys[i]] > reasons[keys[j]] })
	for _, k := range keys {
		fmt.Printf("%6d  %s\n", reasons[k], k)
	}
}
