//go:build !windows

package lite

import "errors"

func Run() error {
	return errors.New("LSYL Tunnel Lite is only available on Windows")
}
