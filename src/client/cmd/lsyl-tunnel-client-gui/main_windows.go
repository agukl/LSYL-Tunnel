//go:build windows

package main

import (
	"log"
	"os"

	"lsyltunnel/src/client/gui"

	"github.com/lxn/walk"
)

func main() {
	if err := gui.RunFromArgs(os.Args[1:]); err != nil {
		log.Print(err)
		if !gui.IsQuitCommand(os.Args[1:]) {
			walk.MsgBox(nil, "LSYL Tunnel Client", err.Error(), walk.MsgBoxIconError)
		}
		os.Exit(1)
	}
}
