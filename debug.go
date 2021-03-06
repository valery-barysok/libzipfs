// +build debug

// To get FUSE debug log activated, build with `go build -tags debug`.

package libzipfs

import (
	"log"

	"bazil.org/fuse"
)

func init() {
	fuse.Debug = func(msg interface{}) {
		log.Print(msg)
	}
}
