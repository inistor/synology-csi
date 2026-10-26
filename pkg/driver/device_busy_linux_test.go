package driver

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestDeviceInUseOnRealDevices checks deviceInUse against the kernel: a loop
// device carrying btrfs, mounted directly and through a device-mapper map as
// multipath would build. It needs root, losetup, mkfs.btrfs and dmsetup, so it
// runs only when asked, e.g. in a privileged container:
//
//	SYNOLOGY_CSI_DEVICE_TESTS=1 go test -run TestDeviceInUseOnRealDevices ./pkg/driver/
func TestDeviceInUseOnRealDevices(t *testing.T) {
	if os.Getenv("SYNOLOGY_CSI_DEVICE_TESTS") != "1" || os.Geteuid() != 0 {
		t.Skip("set SYNOLOGY_CSI_DEVICE_TESTS=1 and run as root")
	}
	run := func(name string, args ...string) string {
		t.Helper()
		out, err := exec.Command(name, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("%s %v: %v\n%s", name, args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	check := func(dev string, want bool, state string) {
		t.Helper()
		inUse, err := deviceInUse(dev)
		if err != nil {
			t.Fatalf("%s: %v", state, err)
		}
		if inUse != want {
			t.Errorf("%s: deviceInUse(%s) = %v, want %v", state, dev, inUse, want)
		}
	}

	dir := t.TempDir()
	image, mnt := filepath.Join(dir, "disk.img"), filepath.Join(dir, "mnt")
	if err := os.Mkdir(mnt, 0o755); err != nil {
		t.Fatal(err)
	}
	run("truncate", "-s", "256M", image)
	loop := run("losetup", "--find", "--show", image)
	t.Cleanup(func() { exec.Command("losetup", "-d", loop).Run() })
	run("mkfs.btrfs", "-q", loop)

	check(loop, false, "formatted, not mounted")
	run("mount", loop, mnt)
	check(loop, true, "mounted")
	run("umount", mnt)
	check(loop, false, "unmounted")

	sectors := run("blockdev", "--getsz", loop)
	run("dmsetup", "create", "csi-test", "--table", "0 "+sectors+" linear "+loop+" 0")
	t.Cleanup(func() { exec.Command("dmsetup", "remove", "csi-test").Run() })
	run("udevadm", "settle")
	mapper := "/dev/mapper/csi-test"
	if _, err := os.Stat(mapper); err != nil {
		run("dmsetup", "mknodes", "csi-test")
	}

	check(loop, false, "held by an unmounted map")
	check(mapper, false, "map not mounted")
	run("mount", mapper, mnt)
	t.Cleanup(func() { exec.Command("umount", mnt).Run() })
	check(loop, true, "held by a mounted map")
	check(mapper, true, "map mounted")
}
