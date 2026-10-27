package driver

import "sync"

// inFlight holds the keys of the controller operations still running.
//
// csi-provisioner gives up on a call after its --timeout and calls again, but
// the first call keeps running here. Two creates of one volume would then share
// its LUN and target, and the one that failed would roll back what the other
// had just returned. A second call for a busy key is refused with Aborted
// instead, as the CSI spec asks, and the sidecar retries it later.
type inFlight struct {
	mu   sync.Mutex
	keys map[string]struct{}
}

// begin claims key and reports whether it was free. A caller that got true
// must call end.
func (f *inFlight) begin(key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, busy := f.keys[key]; busy {
		return false
	}
	if f.keys == nil {
		f.keys = map[string]struct{}{}
	}
	f.keys[key] = struct{}{}
	return true
}

func (f *inFlight) end(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.keys, key)
}
