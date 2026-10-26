package driver

import (
	"errors"
	"fmt"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// deviceInUseBy decides whether a block device is still in use from an
// exclusive open, which fails with EBUSY while the device is mounted or claimed.
//
// A claim alone isn't use: multipath claims every path device it maps, including
// a single path that the driver addresses directly. So a claimed device counts
// as in use only if one of its holders is, or if it has no holder, which leaves
// a mount as the reason.
func deviceInUseBy(devPath string, openExclusive func(string) error, holders func(string) ([]string, error)) (bool, error) {
	resolved, err := filepath.EvalSymlinks(devPath)
	if err != nil {
		return false, err
	}
	err = openExclusive(resolved)
	if err == nil {
		return false, nil
	}
	if !errors.Is(err, unix.EBUSY) {
		return false, err
	}

	held, err := holders(resolved)
	if err != nil {
		return true, fmt.Errorf("device %s is busy and its holders could not be read: %w", resolved, err)
	}
	if len(held) == 0 {
		return true, nil
	}
	for _, holder := range held {
		inUse, err := deviceInUseBy(holder, openExclusive, holders)
		if inUse || err != nil {
			return inUse, err
		}
	}
	return false, nil
}
