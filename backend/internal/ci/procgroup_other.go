//go:build !unix

package ci

import "os/exec"

func ownGroup(*exec.Cmd) {}
