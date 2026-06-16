//go:build windows

package main

import (
	"log"
	"os"
	"strings"

	"lsyltunnel/src/server/gui"

	"github.com/lxn/walk"
)

func main() {
	if err := gui.RunFromArgs(os.Args[1:]); err != nil {
		log.Print(err)
		if !isNonInteractiveAction(os.Args[1:]) {
			walk.MsgBox(nil, "LSYL Tunnel Server", err.Error(), walk.MsgBoxIconError)
		}
		os.Exit(1)
	}
}

func isNonInteractiveAction(args []string) bool {
	for _, arg := range args {
		if arg == "-service-action" || strings.HasPrefix(arg, "-service-action=") ||
			arg == "-config-compat-check" || strings.HasPrefix(arg, "-config-compat-check=") {
			return true
		}
	}
	return false
}
