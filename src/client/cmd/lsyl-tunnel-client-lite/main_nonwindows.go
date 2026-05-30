//go:build !windows

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "LSYL Tunnel Lite is only available on Windows.")
	os.Exit(1)
}
