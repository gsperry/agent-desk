// Command demogen writes the made-up demo sessions that agent-desk --demo
// serves, for the website's screenshots and video:
//
//	go run ./tools/demogen /tmp/site-demo/desk
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/gsperry/agent-desk/internal/demo"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: demogen <dir>")
		os.Exit(2)
	}
	if err := demo.Write(os.Args[1], time.Now()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("ok", os.Args[1])
}
