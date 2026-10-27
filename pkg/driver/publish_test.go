package driver

import (
	"context"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/inistor/synology-csi/pkg/models"
)

// volumeService answers GetVolume with a fixed volume, or nil for "gone".
type volumeService struct {
	lookupFailingService
	volume *models.K8sVolumeRespSpec
}

func (s *volumeService) GetVolume(volId string) (*models.K8sVolumeRespSpec, error) {
	return s.volume, nil
}

func publishRequest() *csi.ControllerPublishVolumeRequest {
	return &csi.ControllerPublishVolumeRequest{
		VolumeId: "vol-1",
		NodeId:   "node-1",
		VolumeCapability: &csi.VolumeCapability{
			AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
			AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
		},
	}
}

func wantCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	if got := status.Code(err); got != want {
		t.Errorf("got %v (%v), want %v", got, err, want)
	}
}

// The capability is what makes csi-provisioner wait for VolumeAttachments to go
// before it deletes a volume.
func TestDriverAdvertisesPublishUnpublish(t *testing.T) {
	d, err := NewControllerAndNodeDriver(DriverName, "node-1", "unix:///csi.sock", &lookupFailingService{}, NewTools(&recordingExecutor{}))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range d.csCap {
		if c.GetRpc().GetType() == csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME {
			return
		}
	}
	t.Error("PUBLISH_UNPUBLISH_VOLUME not advertised")
}

func TestPublishExistingVolumeSucceeds(t *testing.T) {
	cs := &controllerServer{Driver: testDriver(), dsmService: &volumeService{volume: &models.K8sVolumeRespSpec{VolumeId: "vol-1"}}}
	if _, err := cs.ControllerPublishVolume(context.Background(), publishRequest()); err != nil {
		t.Fatal(err)
	}
}

func TestPublishMissingVolumeIsNotFound(t *testing.T) {
	cs := &controllerServer{Driver: testDriver(), dsmService: &volumeService{}}
	_, err := cs.ControllerPublishVolume(context.Background(), publishRequest())
	wantCode(t, err, codes.NotFound)
}

func TestPublishLookupFailureIsRetried(t *testing.T) {
	cs := &controllerServer{Driver: testDriver(), dsmService: &lookupFailingService{}}
	_, err := cs.ControllerPublishVolume(context.Background(), publishRequest())
	wantUnavailable(t, err)
}

func TestPublishValidatesArguments(t *testing.T) {
	cs := &controllerServer{Driver: testDriver(), dsmService: &volumeService{}}
	for name, mutate := range map[string]func(*csi.ControllerPublishVolumeRequest){
		"no volume":     func(r *csi.ControllerPublishVolumeRequest) { r.VolumeId = "" },
		"no node":       func(r *csi.ControllerPublishVolumeRequest) { r.NodeId = "" },
		"no capability": func(r *csi.ControllerPublishVolumeRequest) { r.VolumeCapability = nil },
	} {
		req := publishRequest()
		mutate(req)
		_, err := cs.ControllerPublishVolume(context.Background(), req)
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("%s: got %v, want InvalidArgument", name, err)
		}
	}
}

// Detach must never get stuck on a volume that is already gone.
func TestUnpublishSucceedsForAMissingVolume(t *testing.T) {
	cs := &controllerServer{Driver: testDriver(), dsmService: &volumeService{}}
	if _, err := cs.ControllerUnpublishVolume(context.Background(), &csi.ControllerUnpublishVolumeRequest{VolumeId: "gone", NodeId: "node-1"}); err != nil {
		t.Fatal(err)
	}
	_, err := cs.ControllerUnpublishVolume(context.Background(), &csi.ControllerUnpublishVolumeRequest{})
	wantCode(t, err, codes.InvalidArgument)
}
