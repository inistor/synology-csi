package service

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/inistor/synology-csi/pkg/dsm/webapi"
	"github.com/inistor/synology-csi/pkg/models"
	"github.com/inistor/synology-csi/pkg/utils"
)

// iscsiDeleteAPI is the part of the DSM API that deleting an iSCSI volume uses.
type iscsiDeleteAPI interface {
	LunGet(uuid string) (webapi.LunInfo, error)
	LunDelete(uuid string) error
	TargetList() ([]webapi.TargetInfo, error)
	TargetDelete(targetId string) error
}

// deleteIscsiVolume removes a volume's target, then its LUN.
//
// The order is what makes a retry safe. Volumes are found through their
// targets' LUN mappings, so a target whose LUN is already gone maps nothing and
// nothing finds it again: a retry would report the volume deleted and the target
// would count against the DSM's target limit forever. A LUN whose target is gone
// is still found by its UUID, through findUnmappedLun.
func deleteIscsiVolume(api iscsiDeleteAPI, dsmIp string, lun webapi.LunInfo, target webapi.TargetInfo) error {
	if len(target.MappedLuns) == 1 {
		targetId := strconv.Itoa(target.TargetId)
		if err := api.TargetDelete(targetId); err != nil {
			gone, listErr := targetGone(api, target.TargetId)
			if !gone {
				log.Errorf("[%s] Failed to delete target(%s): %v", dsmIp, targetId, errors.Join(err, listErr))
				return err
			}
		}
	} else {
		log.Infof("[%s] Skip deleting target[%s], which maps %d LUNs", dsmIp, target.Name, len(target.MappedLuns))
	}

	if err := api.LunDelete(lun.Uuid); err != nil {
		if _, getErr := api.LunGet(lun.Uuid); isNoSuchLun(getErr) {
			return nil
		}
		log.Errorf("[%s] Failed to delete LUN(%s): %v", dsmIp, lun.Uuid, err)
		return err
	}
	return nil
}

// targetGone reports whether a target is known to be absent. Only a complete
// listing proves that; a failed lookup proves nothing.
func targetGone(api iscsiDeleteAPI, targetId int) (bool, error) {
	targets, err := api.TargetList()
	if err != nil {
		return false, err
	}
	for _, target := range targets {
		if target.TargetId == targetId {
			return false, nil
		}
	}
	return true, nil
}

// findUnmappedLun looks a LUN up by UUID on each DSM, for a volume that the
// target-based listing no longer shows: one whose deletion removed the target
// but failed before the LUN. It returns (nil, nil) only when every DSM says
// there is no such LUN. LUNs not named like this driver's are ignored.
func findUnmappedLun[API iscsiDeleteAPI](dsms map[string]API, volId string) (API, *webapi.LunInfo, error) {
	var zero API
	var lookupErr error
	for ip, api := range dsms {
		lun, err := api.LunGet(volId)
		if err != nil {
			if !isNoSuchLun(err) {
				lookupErr = errors.Join(lookupErr, fmt.Errorf("DSM[%s] failed to get LUN(%s): %w", ip, volId, err))
			}
			continue
		}
		if lun.Uuid != volId || !strings.HasPrefix(lun.Name, models.DevicePrefix) {
			continue
		}
		return api, &lun, nil
	}
	return zero, nil, lookupErr
}

// isNoSuchLun reports whether a LUN lookup failed because no LUN has that UUID.
// DSM answers a well-formed UUID it doesn't know with "no such LUN" and anything
// else with "invalid parameter" (18990505); neither can be a LUN.
func isNoSuchLun(err error) bool {
	return errors.Is(err, utils.NoSuchLunError("")) || errors.Is(err, utils.IscsiDefaultError{ErrCode: 18990505})
}
