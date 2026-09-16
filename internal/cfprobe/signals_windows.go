//go:build windows

package cfprobe

import "os"

func shutdownSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}
