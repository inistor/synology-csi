package driver

import (
	"strings"
	"testing"
)

const timeoutTestIqn = "iqn.2000-01.com.synology:k8s-csi.pvc-aaa"

func TestLoginSetsReplacementTimeoutBeforeLoggingIn(t *testing.T) {
	e := &recordingExecutor{}
	d := &initiatorDriver{tools: NewTools(e)}
	d.tools.ISCSIReplacementTimeout = 300

	if err := d.login(timeoutTestIqn, "10.10.10.20:3260"); err != nil {
		t.Fatalf("login: %v", err)
	}

	update := e.find("node.session.timeo.replacement_timeout --value 300")
	login := e.find("--login")
	if update < 0 {
		t.Fatalf("replacement timeout was never set; ran %q", e.ran)
	}
	// The session reads the record at login: set afterwards, the record says 300
	// while the live session keeps the old value.
	if login < update || e.find("--op new") > update {
		t.Errorf("want --op new, then the timeout update, then --login; ran %q", e.ran)
	}
}

func TestLoginLeavesReplacementTimeoutAloneWhenUnset(t *testing.T) {
	e := &recordingExecutor{}
	d := &initiatorDriver{tools: NewTools(e)}

	if err := d.login(timeoutTestIqn, "10.10.10.20:3260"); err != nil {
		t.Fatalf("login: %v", err)
	}

	if i := e.find("replacement_timeout"); i >= 0 {
		t.Errorf("touched the replacement timeout without being asked: %q", e.ran[i])
	}
}

func TestLoginRefusesWhenReplacementTimeoutCannotBeSet(t *testing.T) {
	e := &recordingExecutor{fail: "replacement_timeout"}
	d := &initiatorDriver{tools: NewTools(e)}
	d.tools.ISCSIReplacementTimeout = 300

	err := d.login(timeoutTestIqn, "10.10.10.20:3260")

	if err == nil {
		t.Fatal("logged in although the replacement timeout could not be set")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error should carry iscsiadm's output, got %v", err)
	}
	if i := e.find("--login"); i >= 0 {
		t.Errorf("logged in on the node default timeout: %q", e.ran)
	}
}
