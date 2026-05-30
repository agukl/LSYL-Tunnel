//go:build windows

package main

import (
	"log"
	"os"

	"lsyltunnel/src/client/lite"

	"github.com/lxn/walk"
)

func main() {
	if err := lite.Run(); err != nil {
		log.Print(err)
		walk.MsgBox(nil, "LSYL Tunnel Lite", err.Error(), walk.MsgBoxIconError)
		os.Exit(1)
	}
}
