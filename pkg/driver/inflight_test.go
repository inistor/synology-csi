package driver

import (
	"context"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// slowDeleteService holds DeleteVolume until released, like a DSM slower than
// the sidecar's timeout.
type slowDeleteService struct {
	lookupFailingService
	entered, release chan struct{}
}

func (s *slowDeleteService) DeleteVolume(volId string) error {
	s.entered <- struct{}{}
	<-s.release
	return nil
}

func TestOverlappingCallForTheSameVolumeIsAborted(t *testing.T) {
	svc := &slowDeleteService{entered: make(chan struct{}), release: make(chan struct{})}
	cs := &controllerServer{Driver: testDriver(), dsmService: svc}
	req := &csi.DeleteVolumeRequest{VolumeId: "vol-1"}

	first := make(chan error)
	go func() {
		_, err := cs.DeleteVolume(context.Background(), req)
		first <- err
	}()
	<-svc.entered

	if _, err := cs.DeleteVolume(context.Background(), req); status.Code(err) != codes.Aborted {
		t.Errorf("overlapping call: err = %v, want Aborted", err)
	}
	if _, err := cs.ControllerExpandVolume(context.Background(), &csi.ControllerExpandVolumeRequest{
		VolumeId: "vol-1", CapacityRange: &csi.CapacityRange{RequiredBytes: 1 << 30},
	}); status.Code(err) != codes.Aborted {
		t.Errorf("expand during delete: err = %v, want Aborted", err)
	}

	close(svc.release)
	if err := <-first; err != nil {
		t.Fatalf("first call: %v", err)
	}

	// The retry, once the first call is done, goes through.
	go func() { <-svc.entered }()
	if _, err := cs.DeleteVolume(context.Background(), req); err != nil {
		t.Errorf("retry after the first call ended: %v", err)
	}
}

func TestInFlightKeysAreIndependent(t *testing.T) {
	var f inFlight
	if !f.begin("a") || !f.begin("b") {
		t.Fatal("distinct keys must not block each other")
	}
	if f.begin("a") {
		t.Error("a busy key was claimed twice")
	}
	f.end("a")
	if !f.begin("a") {
		t.Error("a released key could not be claimed again")
	}
}
