package driver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/mount-utils"
)

// EBUSY (still mounted) and ENOTEMPTY (files left behind) point at different
// bugs, so the error must say which one stopped the target path's removal.
func TestNodeUnpublishReportsWhyTargetPathRemovalFailed(t *testing.T) {
	targetPath := mountTempDir(t)
	if err := os.WriteFile(filepath.Join(targetPath, "leftover"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	ns := &nodeServer{
		Driver: testDriver(),
		Mounter: &mount.SafeFormatAndMount{Interface: mount.NewFakeMounter([]mount.MountPoint{
			{Device: "/dev/sdx", Path: targetPath, Type: "btrfs"},
		})},
	}

	_, err := ns.NodeUnpublishVolume(context.Background(), &csi.NodeUnpublishVolumeRequest{
		VolumeId:   "vol-1",
		TargetPath: targetPath,
	})

	if status.Code(err) != codes.Internal {
		t.Fatalf("got %v, want an Internal error", err)
	}
	if !strings.Contains(err.Error(), syscall.ENOTEMPTY.Error()) {
		t.Errorf("error %q does not say why the removal failed", err)
	}
}
