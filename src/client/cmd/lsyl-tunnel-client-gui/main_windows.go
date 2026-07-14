//go:build windows

package main

import (
	"log"
	"os"

	"lsyltunnel/src/client/gui"
	"lsyltunnel/src/client/tunnel"

	"github.com/lxn/walk"
)

func main() {
	if handled, err := tunnel.HandleVirtualRedirectHelperArgs(os.Args[1:]); handled {
		if err != nil {
			log.Print(err)
			os.Exit(1)
		}
		return
	}
	if err := gui.RunFromArgs(os.Args[1:]); err != nil {
		log.Print(err)
		if !gui.IsNonInteractiveCommand(os.Args[1:]) {
			walk.MsgBox(nil, "LSYL Tunnel Client", err.Error(), walk.MsgBoxIconError)
		}
		os.Exit(1)
	}
}
