package driver

import (
	"fmt"
	"os"
	"path/filepath"

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

// deviceInUse reports whether something on the host still uses a block device:
// a mount, or a holder such as a device-mapper map that is itself in use.
func deviceInUse(devPath string) (bool, error) {
	return deviceInUseBy(devPath, openExclusive, deviceHolders)
}

// openExclusive opens a block device with O_EXCL, which the kernel refuses with
// EBUSY while the device is mounted or claimed by a holder.
func openExclusive(devPath string) error {
	f, err := os.OpenFile(devPath, os.O_RDONLY|unix.O_EXCL, 0)
	if err != nil {
		return err
	}
	return f.Close()
}

// deviceHolders lists the devices claiming a block device. It finds the device
// in sysfs by its number, as /dev/mapper names need not be links to dm-N.
func deviceHolders(devPath string) ([]string, error) {
	var st unix.Stat_t
	if err := unix.Stat(devPath, &st); err != nil {
		return nil, err
	}
	sysDir := fmt.Sprintf("/sys/dev/block/%d:%d/holders", unix.Major(st.Rdev), unix.Minor(st.Rdev))
	entries, err := os.ReadDir(sysDir)
	if err != nil {
		return nil, err
	}
	holders := make([]string, 0, len(entries))
	for _, entry := range entries {
		holders = append(holders, filepath.Join("/dev", entry.Name()))
	}
	return holders, nil
}
