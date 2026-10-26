//go:build !linux

package driver

import "errors"

// flushDeviceBuffers needs the Linux BLKFLSBUF ioctl; this stub only keeps the
// package building on other systems for development.
func (t *tools) flushDeviceBuffers(devPath string) error {
	return errors.New("flushing device buffers is only supported on Linux")
}
