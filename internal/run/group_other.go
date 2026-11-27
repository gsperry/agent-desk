//go:build !unix

package run

import "os/exec"

func ownGroup(*exec.Cmd) {}
