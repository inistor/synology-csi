package driver

import (
	"os"

	"golang.org/x/sys/unix"
)

// flushDeviceBuffers writes out anything the kernel still holds for a block
// device and drops its buffers, the same BLKFLSBUF ioctl that
// `blockdev --flushbufs` makes.
//
// Raw block volumes are written through the page cache with no filesystem to
// flush them, so the data reaches the LUN only when the last opener closes it.
// Nothing in unstage waited for that before tearing the session down, which
// leaves the write racing the next node's read. The in-tree iSCSI plugin does
// the same thing on its detach path. It is a no-op on a device that is already
// clean.
//
// The ioctl runs in the plugin, on the host's /dev, rather than as a host
// command, so it works on a host that has no blockdev binary.
func (t *tools) flushDeviceBuffers(devPath string) error {
	f, err := os.OpenFile(devPath, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	return unix.IoctlSetInt(int(f.Fd()), unix.BLKFLSBUF, 0)
}
