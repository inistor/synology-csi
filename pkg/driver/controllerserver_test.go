/*
Copyright 2021 Synology Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package driver

import (
	"context"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/SynologyOpenSource/synology-csi/pkg/utils"
)

// Share-backed protocols serve files and cannot expose a raw block device;
// LUN-backed ones can. volumeMode: Block is rejected based on this split.
func TestIsShareProtocol(t *testing.T) {
	tests := []struct {
		protocol string
		want     bool
	}{
		{utils.ProtocolNfs, true},
		{utils.ProtocolSmb, true},
		{utils.ProtocolIscsi, false},
		{utils.ProtocolNvme, false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.protocol, func(t *testing.T) {
			if got := isShareProtocol(tt.protocol); got != tt.want {
				t.Errorf("isShareProtocol(%q) = %v, want %v", tt.protocol, got, tt.want)
			}
		})
	}
}

// Several nodes mounting one LUN's local filesystem read-write corrupts it, so
// ReadWriteMany is only safe on a share or as a raw block device.
func TestIsUnsafeMultiNodeFilesystem(t *testing.T) {
	mount := &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}}
	block := &csi.VolumeCapability_Block{Block: &csi.VolumeCapability_BlockVolume{}}
	capability := func(mode csi.VolumeCapability_AccessMode_Mode, accessType bool) *csi.VolumeCapability {
		c := &csi.VolumeCapability{AccessMode: &csi.VolumeCapability_AccessMode{Mode: mode}}
		if accessType {
			c.AccessType = mount
		} else {
			c.AccessType = block
		}
		return c
	}
	const isMount, isBlock = true, false
	rwx := csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER
	rwo := csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER

	tests := []struct {
		name     string
		cap      *csi.VolumeCapability
		protocol string
		want     bool
	}{
		{"iSCSI RWX filesystem", capability(rwx, isMount), utils.ProtocolIscsi, true},
		{"NVMe RWX filesystem", capability(rwx, isMount), utils.ProtocolNvme, true},
		{"iSCSI RWX block", capability(rwx, isBlock), utils.ProtocolIscsi, false},
		{"iSCSI RWO filesystem", capability(rwo, isMount), utils.ProtocolIscsi, false},
		{"NFS RWX filesystem", capability(rwx, isMount), utils.ProtocolNfs, false},
		{"SMB RWX filesystem", capability(rwx, isMount), utils.ProtocolSmb, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isUnsafeMultiNodeFilesystem(tt.cap, tt.protocol); got != tt.want {
				t.Errorf("isUnsafeMultiNodeFilesystem() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCreateVolumeRejectsMultiNodeFilesystemOnIscsi(t *testing.T) {
	svc := &lookupFailingService{}
	cs := &controllerServer{Driver: testDriver(), dsmService: svc}

	_, err := cs.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name:       "pvc-1",
		Parameters: map[string]string{"protocol": utils.ProtocolIscsi},
		VolumeCapabilities: []*csi.VolumeCapability{{
			AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{FsType: "btrfs"}},
			AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER},
		}},
	})

	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("got %v, want InvalidArgument", err)
	}
	if svc.createVolumeCalled {
		t.Error("created a LUN that several nodes would mount read-write")
	}
}
