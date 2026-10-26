package driver

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

// The flush runs in the plugin, so a host with no blockdev binary still gets
// it. A regular file is not a block device, so the ioctl reaching it proves
// the flush was attempted here and not handed to the host.
func TestFlushRunsInThePluginWithNoHostCommand(t *testing.T) {
	e := &recordingExecutor{}
	tools := NewTools(e)
	notABlockDevice := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(notABlockDevice, nil, 0600); err != nil {
		t.Fatal(err)
	}

	err := tools.flushDeviceBuffers(notABlockDevice)

	if !errors.Is(err, unix.ENOTTY) {
		t.Errorf("flushing a regular file returned %v, want ENOTTY from the ioctl", err)
	}
	if len(e.ran) != 0 {
		t.Errorf("expected no host command, ran: %v", e.ran)
	}
}
