package service

import (
	"errors"
	"strconv"
	"testing"

	"github.com/inistor/synology-csi/pkg/dsm/webapi"
	"github.com/inistor/synology-csi/pkg/utils"
)

// fakeIscsi is a DSM holding LUNs and targets. A target's mapping goes when its
// LUN does, as on DSM.
type fakeIscsi struct {
	luns    map[string]webapi.LunInfo
	targets map[int]webapi.TargetInfo

	failTargetDelete, failLunDelete, failList, failLunGet error
	calls                                                 []string
}

func newFakeIscsi() *fakeIscsi {
	f := &fakeIscsi{luns: map[string]webapi.LunInfo{}, targets: map[int]webapi.TargetInfo{}}
	f.luns["uuid-1"] = webapi.LunInfo{Uuid: "uuid-1", Name: "k8s-csi-pvc-1"}
	f.targets[7] = webapi.TargetInfo{TargetId: 7, Name: "k8s-csi-pvc-1", MappedLuns: []webapi.MappedLun{{LunUuid: "uuid-1"}}}
	return f
}

func (f *fakeIscsi) LunGet(uuid string) (webapi.LunInfo, error) {
	if f.failLunGet != nil {
		return webapi.LunInfo{}, f.failLunGet
	}
	lun, ok := f.luns[uuid]
	if !ok {
		return webapi.LunInfo{}, utils.NoSuchLunError("")
	}
	return lun, nil
}

func (f *fakeIscsi) LunDelete(uuid string) error {
	f.calls = append(f.calls, "LunDelete")
	if f.failLunDelete != nil {
		return f.failLunDelete
	}
	if _, ok := f.luns[uuid]; !ok {
		return utils.NoSuchLunError("")
	}
	delete(f.luns, uuid)
	for id, target := range f.targets {
		var kept []webapi.MappedLun
		for _, m := range target.MappedLuns {
			if m.LunUuid != uuid {
				kept = append(kept, m)
			}
		}
		target.MappedLuns = kept
		f.targets[id] = target
	}
	return nil
}

func (f *fakeIscsi) TargetList() ([]webapi.TargetInfo, error) {
	if f.failList != nil {
		return nil, f.failList
	}
	var out []webapi.TargetInfo
	for _, target := range f.targets {
		out = append(out, target)
	}
	return out, nil
}

func (f *fakeIscsi) TargetDelete(targetId string) error {
	f.calls = append(f.calls, "TargetDelete")
	if f.failTargetDelete != nil {
		return f.failTargetDelete
	}
	id, _ := strconv.Atoi(targetId)
	if _, ok := f.targets[id]; !ok {
		return errors.New("no such target")
	}
	delete(f.targets, id)
	return nil
}

// volume returns what the target-based listing would find for uuid-1, or nil.
func (f *fakeIscsi) volume() (webapi.LunInfo, webapi.TargetInfo, bool) {
	for _, target := range f.targets {
		for _, m := range target.MappedLuns {
			if lun, ok := f.luns[m.LunUuid]; ok {
				return lun, target, true
			}
		}
	}
	return webapi.LunInfo{}, webapi.TargetInfo{}, false
}

func TestDeleteRemovesTargetBeforeLun(t *testing.T) {
	f := newFakeIscsi()
	lun, target, _ := f.volume()

	if err := deleteIscsiVolume(f, "dsm", lun, target); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 2 || f.calls[0] != "TargetDelete" || f.calls[1] != "LunDelete" {
		t.Errorf("calls = %v, want target then LUN", f.calls)
	}
	if len(f.luns) != 0 || len(f.targets) != 0 {
		t.Errorf("left behind: %d LUNs, %d targets", len(f.luns), len(f.targets))
	}
}

func TestFailedTargetDeleteKeepsTheVolumeFindable(t *testing.T) {
	f := newFakeIscsi()
	f.failTargetDelete = errors.New("DSM busy")
	lun, target, _ := f.volume()

	if err := deleteIscsiVolume(f, "dsm", lun, target); err == nil {
		t.Fatal("expected an error")
	}
	// The LUN must still be there, or the target would map nothing and a retry
	// would never find it.
	if _, _, ok := f.volume(); !ok {
		t.Error("volume is no longer findable after a failed target delete")
	}
}

func TestFailedTargetDeleteIsNotTakenAsGoneWhenTheListingFails(t *testing.T) {
	f := newFakeIscsi()
	f.failTargetDelete = errors.New("timeout")
	f.failList = errors.New("timeout")
	lun, target, _ := f.volume()

	if err := deleteIscsiVolume(f, "dsm", lun, target); err == nil {
		t.Fatal("expected an error")
	}
	if _, ok := f.luns["uuid-1"]; !ok {
		t.Error("LUN deleted although the target may still exist")
	}
}

func TestTargetAlreadyGoneStillDeletesLun(t *testing.T) {
	f := newFakeIscsi()
	lun, target, _ := f.volume()
	delete(f.targets, 7)

	if err := deleteIscsiVolume(f, "dsm", lun, target); err != nil {
		t.Fatal(err)
	}
	if len(f.luns) != 0 {
		t.Error("LUN left behind")
	}
}

func TestRetryAfterFailedLunDeleteFindsAndRemovesTheLun(t *testing.T) {
	f := newFakeIscsi()
	f.failLunDelete = errors.New("DSM busy")
	lun, target, _ := f.volume()

	if err := deleteIscsiVolume(f, "dsm", lun, target); err == nil {
		t.Fatal("expected an error")
	}
	if _, _, ok := f.volume(); ok {
		t.Fatal("test setup: the listing should no longer show the volume")
	}

	// The retry: the listing misses it, the direct lookup must not.
	f.failLunDelete = nil
	api, orphan, err := findUnmappedLun(map[string]*fakeIscsi{"dsm": f}, "uuid-1")
	if err != nil || orphan == nil {
		t.Fatalf("orphaned LUN not found: lun=%v err=%v", orphan, err)
	}
	if err := deleteIscsiVolume(api, "dsm", *orphan, webapi.TargetInfo{}); err != nil {
		t.Fatal(err)
	}
	if len(f.luns) != 0 || len(f.targets) != 0 {
		t.Errorf("left behind: %d LUNs, %d targets", len(f.luns), len(f.targets))
	}
}

func TestFindUnmappedLun(t *testing.T) {
	f := newFakeIscsi()
	f.luns["uuid-other"] = webapi.LunInfo{Uuid: "uuid-other", Name: "webui-db"}
	dsms := map[string]*fakeIscsi{"dsm": f}

	if _, lun, err := findUnmappedLun(dsms, "uuid-missing"); lun != nil || err != nil {
		t.Errorf("missing volume: lun=%v err=%v, want neither", lun, err)
	}
	if _, lun, err := findUnmappedLun(dsms, "uuid-other"); lun != nil || err != nil {
		t.Errorf("a LUN this driver didn't name must be ignored: lun=%v err=%v", lun, err)
	}

	f.failLunGet = utils.IscsiDefaultError{ErrCode: 18990505}
	if _, lun, err := findUnmappedLun(dsms, "not-a-uuid"); lun != nil || err != nil {
		t.Errorf("invalid parameter must read as no such LUN: lun=%v err=%v", lun, err)
	}

	// An unreachable DSM proves nothing, so the deletion must not be reported done.
	f.failLunGet = errors.New("connection refused")
	if _, lun, err := findUnmappedLun(dsms, "uuid-missing"); lun != nil || err == nil {
		t.Errorf("unreachable DSM: lun=%v err=%v, want an error", lun, err)
	}
}

func TestTargetMappingOtherLunsIsKept(t *testing.T) {
	f := newFakeIscsi()
	f.luns["uuid-2"] = webapi.LunInfo{Uuid: "uuid-2", Name: "k8s-csi-pvc-2"}
	target := f.targets[7]
	target.MappedLuns = append(target.MappedLuns, webapi.MappedLun{LunUuid: "uuid-2"})
	f.targets[7] = target

	if err := deleteIscsiVolume(f, "dsm", f.luns["uuid-1"], target); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.targets[7]; !ok {
		t.Error("a target still mapping another LUN was deleted")
	}
	if _, ok := f.luns["uuid-1"]; ok {
		t.Error("LUN not deleted")
	}
}

// Another volume's LUN deleted between the target list and its lookup must not
// make the listing incomplete, or every DeleteVolume running alongside fails.
func TestListingSkipsALunDeletedMeanwhile(t *testing.T) {
	f := newFakeIscsi()
	f.targets[8] = webapi.TargetInfo{TargetId: 8, Name: "k8s-csi-pvc-gone", MappedLuns: []webapi.MappedLun{{LunUuid: "uuid-gone"}}}

	infos, err := listDsmISCSIVolumes(f, "dsm")
	if err != nil {
		t.Fatalf("a LUN that no longer exists made the listing fail: %v", err)
	}
	if len(infos) != 1 || infos[0].Lun.Uuid != "uuid-1" {
		t.Errorf("listed %v, want only uuid-1", infos)
	}

	f.failLunGet = errors.New("connection refused")
	if _, err := listDsmISCSIVolumes(f, "dsm"); err == nil {
		t.Error("an unreachable DSM must leave the listing incomplete")
	}
}
