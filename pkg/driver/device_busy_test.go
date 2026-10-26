package driver

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

// fakeDevices stands in for /dev: busy devices answer an exclusive open with
// EBUSY, and holders maps a device to the devices claiming it.
type fakeDevices struct {
	dir     string
	busy    map[string]bool
	holders map[string][]string
}

func newFakeDevices(t *testing.T, names ...string) *fakeDevices {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return &fakeDevices{dir: dir, busy: map[string]bool{}, holders: map[string][]string{}}
}

func (f *fakeDevices) path(name string) string { return filepath.Join(f.dir, name) }

func (f *fakeDevices) open(path string) error {
	if f.busy[filepath.Base(path)] {
		return &os.PathError{Op: "open", Path: path, Err: unix.EBUSY}
	}
	return nil
}

func (f *fakeDevices) holdersOf(path string) ([]string, error) {
	var out []string
	for _, h := range f.holders[filepath.Base(path)] {
		out = append(out, f.path(h))
	}
	return out, nil
}

func (f *fakeDevices) inUse(t *testing.T, name string) bool {
	t.Helper()
	inUse, err := deviceInUseBy(f.path(name), f.open, f.holdersOf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return inUse
}

func TestIdleDeviceIsNotInUse(t *testing.T) {
	f := newFakeDevices(t, "sdb")
	if f.inUse(t, "sdb") {
		t.Error("idle device reported in use")
	}
}

func TestMountedDeviceIsInUse(t *testing.T) {
	f := newFakeDevices(t, "sdb")
	f.busy["sdb"] = true // claimed with no holder: a mount
	if !f.inUse(t, "sdb") {
		t.Error("mounted device reported free; logging out would force btrfs read-only")
	}
}

// Multipath claims a single path device the driver addresses directly; that
// claim alone must not block the logout forever.
func TestDeviceHeldOnlyByAnIdleMapIsNotInUse(t *testing.T) {
	f := newFakeDevices(t, "sdb", "dm-3")
	f.busy["sdb"] = true
	f.holders["sdb"] = []string{"dm-3"}
	if f.inUse(t, "sdb") {
		t.Error("device held only by an unmounted multipath map reported in use")
	}
}

func TestDeviceHeldByAMountedMapIsInUse(t *testing.T) {
	f := newFakeDevices(t, "sdb", "dm-3")
	f.busy["sdb"] = true
	f.busy["dm-3"] = true
	f.holders["sdb"] = []string{"dm-3"}
	if !f.inUse(t, "sdb") {
		t.Error("device whose multipath map is mounted reported free")
	}
}

func TestSymlinkIsResolvedBeforeTheCheck(t *testing.T) {
	f := newFakeDevices(t, "sdb")
	f.busy["sdb"] = true
	link := f.path("ip-1.2.3.4:3260-iscsi-iqn-lun-1")
	if err := os.Symlink(f.path("sdb"), link); err != nil {
		t.Fatal(err)
	}
	inUse, err := deviceInUseBy(link, f.open, f.holdersOf)
	if err != nil || !inUse {
		t.Errorf("by-path link to a mounted device: inUse=%v err=%v", inUse, err)
	}
}

func TestOtherOpenErrorsAreReportedNotTakenAsBusy(t *testing.T) {
	f := newFakeDevices(t, "sdb")
	open := func(string) error { return errors.New("permission denied") }
	inUse, err := deviceInUseBy(f.path("sdb"), open, f.holdersOf)
	if inUse || err == nil {
		t.Errorf("inUse=%v err=%v, want false and an error", inUse, err)
	}
}

func TestBusyDeviceWithUnreadableHoldersCountsAsInUse(t *testing.T) {
	f := newFakeDevices(t, "sdb")
	f.busy["sdb"] = true
	holders := func(string) ([]string, error) { return nil, errors.New("no sysfs") }
	inUse, _ := deviceInUseBy(f.path("sdb"), f.open, holders)
	if !inUse {
		t.Error("a busy device whose holders can't be read must be treated as in use")
	}
}
