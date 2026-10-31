package service

import (
	"errors"
	"testing"

	"github.com/inistor/synology-csi/pkg/models"
)

func TestFindSnapshotTellsMissingFromUnknown(t *testing.T) {
	listErr := errors.New("failed to list snapshots of share(k8s-csi-pvc-x): busy")
	snaps := []*models.K8sSnapshotRespSpec{{Uuid: "snap-1"}}
	byUuid := func(uuid string) func(*models.K8sSnapshotRespSpec) bool {
		return func(s *models.K8sSnapshotRespSpec) bool { return s.Uuid == uuid }
	}

	// Found in an incomplete listing: it exists, whatever else is missing.
	if snap, err := findSnapshot(snaps, listErr, byUuid("snap-1")); snap == nil || err != nil {
		t.Errorf("found snapshot: got (%v, %v), want it and no error", snap, err)
	}
	// Absent from a complete listing: known not to exist.
	if snap, err := findSnapshot(snaps, nil, byUuid("snap-2")); snap != nil || err != nil {
		t.Errorf("absent from a complete listing: got (%v, %v), want (nil, nil)", snap, err)
	}
	// Absent from an incomplete listing: unknown, which must not read as gone.
	if _, err := findSnapshot(snaps, listErr, byUuid("snap-2")); !errors.Is(err, listErr) {
		t.Errorf("absent from an incomplete listing: got error %v, want the listing error", err)
	}
}
