//go:build !linux

package driver

import "errors"

// flushDeviceBuffers needs the Linux BLKFLSBUF ioctl; this stub only keeps the
// package building on other systems for development.
func (t *tools) flushDeviceBuffers(devPath string) error {
	return errors.New("flushing device buffers is only supported on Linux")
}

// deviceInUse needs O_EXCL semantics of Linux block devices.
func deviceInUse(devPath string) (bool, error) {
	return false, errors.New("checking whether a device is in use is only supported on Linux")
}
