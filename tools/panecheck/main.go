// Command panecheck reads every running session's screen through
// agent-manager and checks what Agent Desk makes of it, so a change in how
// Claude Code draws a dialog shows up here before it leaves a question
// unanswerable in the UI. It is read-only and prints labels and counts,
// never screen content; -save writes flagged screens to a folder, owner
// only, to be anonymized before they become test fixtures.
//
//	go run ./tools/panecheck
//	go run ./tools/panecheck -save /tmp/screens
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gsperry/agent-desk/internal/am"
	"github.com/gsperry/agent-desk/internal/pane"
	"github.com/gsperry/agent-desk/internal/send"
)

// Signals that a screen holds a question, kept apart from the parser so
// they do not share its blind spots.
var (
	hint     = regexp.MustCompile(`Enter to (?:select|confirm)|Esc to cancel|Tab/Arrow keys to navigate`)
	review   = regexp.MustCompile(`Ready to submit your answers\?`)
	marked   = regexp.MustCompile(`^\s*❯\s*\d{1,2}\.\s`)
	checkbox = regexp.MustCompile(`^\[.\]\s`)
)

func main() {
	save := flag.String("save", "", "write the screens of flagged sessions to this folder")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cli := am.NewCLI()
	snap, err := cli.Snapshot(ctx)
	if err != nil {
		log.Fatal(err)
	}
	if *save != "" {
		if err := os.MkdirAll(*save, 0o700); err != nil {
			log.Fatal(err)
		}
	}
	flagged := 0
	for _, s := range snap.Sessions {
		if !s.Running {
			continue
		}
		screen, err := cli.Read(ctx, s.ID)
		if err != nil {
			fmt.Printf("%-9s %-9s read failed: %v\n", s.ID, s.Status, err)
			continue
		}
		problems, summary := check(s.Status, screen)
		verdict := "ok"
		if len(problems) > 0 {
			verdict = "FLAG " + strings.Join(problems, "; ")
			flagged++
			if *save != "" {
				p := filepath.Join(*save, s.ID+".txt")
				if err := os.WriteFile(p, []byte(screen), 0o600); err != nil {
					log.Fatal(err)
				}
			}
		}
		fmt.Printf("%-9s %-9s %-28s %s\n", s.ID, s.Status, summary, verdict)
	}
	if flagged > 0 {
		if *save != "" {
			fmt.Printf("\nSaved %d screen(s) to %s. Anonymize before committing one as a fixture.\n", flagged, *save)
		}
		os.Exit(1)
	}
}

// check returns what looks wrong with a screen, and a summary of what the
// parser found, both free of screen content.
func check(status, screen string) ([]string, string) {
	lines := strings.Split(screen, "\n")
	tail := lines[max(len(lines)-30, 0):]
	looksAsking := status == "waiting"
	for _, l := range tail {
		if hint.MatchString(l) || review.MatchString(l) || marked.MatchString(l) {
			looksAsking = true
		}
	}
	d := pane.Parse(screen)
	atPrompt := send.AtPrompt(screen)
	var problems []string
	summary := "no dialog"
	if d != nil {
		summary = fmt.Sprintf("dialog: %d options", len(d.Options))
		if d.Multi {
			summary += ", multi"
		}
		if len(d.Steps) > 0 {
			summary += fmt.Sprintf(", %d steps", len(d.Steps))
		}
	} else if atPrompt {
		summary = "at prompt"
	}
	switch {
	case looksAsking && d == nil && !atPrompt:
		problems = append(problems, "looks like a question but nothing parsed")
	case d != nil && len(d.Options) == 0:
		problems = append(problems, "dialog without options")
	}
	if d != nil && len(d.Options) > 0 {
		if strings.TrimSpace(d.Question) == "" {
			problems = append(problems, "no question text")
		}
		hasNext := false
		for _, o := range d.Options {
			if o.Next {
				hasNext = true
			}
			if checkbox.MatchString(o.Label) {
				problems = append(problems, "checkbox left in a label")
				break
			}
		}
		if d.Multi && !hasNext {
			problems = append(problems, "several answers but no Next")
		}
	}
	if d == nil && !atPrompt && (status == "idle" || status == "finished") {
		problems = append(problems, "idle but no prompt box found")
	}
	return problems, summary
}
