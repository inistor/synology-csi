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
	"fmt"
	"reflect"
	"testing"

	utilexec "k8s.io/utils/exec"
	testingexec "k8s.io/utils/exec/testing"
)

func TestParseSessions(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want []iscsiSession
	}{
		{
			name: "two sessions",
			out: `tcp: [1] 10.10.10.20:3260,1 iqn.2000-01.com.synology:k8s-csi.pvc-aaa (non-flash)
tcp: [2] 10.10.10.20:3260,1 iqn.2000-01.com.synology:k8s-csi.pvc-bbb (non-flash)`,
			want: []iscsiSession{
				{Protocol: "tcp", Id: 1, Portal: "10.10.10.20:3260", Iqn: "iqn.2000-01.com.synology:k8s-csi.pvc-aaa", Name: "k8s-csi.pvc-aaa"},
				{Protocol: "tcp", Id: 2, Portal: "10.10.10.20:3260", Iqn: "iqn.2000-01.com.synology:k8s-csi.pvc-bbb", Name: "k8s-csi.pvc-bbb"},
			},
		},
		{
			name: "diagnostics interleaved with sessions",
			out: `tcp: [1] 10.10.10.20:3260,1 iqn.2000-01.com.synology:k8s-csi.pvc-aaa (non-flash)
iscsiadm: could not read session targetname: 5
iscsiadm: could not find session info for session4
tcp: [2] 10.10.10.20:3260,1 iqn.2000-01.com.synology:k8s-csi.pvc-bbb (non-flash)`,
			want: []iscsiSession{
				{Protocol: "tcp", Id: 1, Portal: "10.10.10.20:3260", Iqn: "iqn.2000-01.com.synology:k8s-csi.pvc-aaa", Name: "k8s-csi.pvc-aaa"},
				{Protocol: "tcp", Id: 2, Portal: "10.10.10.20:3260", Iqn: "iqn.2000-01.com.synology:k8s-csi.pvc-bbb", Name: "k8s-csi.pvc-bbb"},
			},
		},
		{
			name: "iqn without a colon",
			out:  `tcp: [1] 10.10.10.20:3260,1 iqn.2000-01.com.synology.k8s-csi.pvc-aaa (non-flash)`,
			want: []iscsiSession{
				{Protocol: "tcp", Id: 1, Portal: "10.10.10.20:3260", Iqn: "iqn.2000-01.com.synology.k8s-csi.pvc-aaa", Name: ""},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSessions(tt.out)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseSessions() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// fakeExecutor records every command it is asked to run and replays scripted
// output for it. CombinedOutput() sees stdout and stderr interleaved, as a real
// command's would be.
type fakeExecutor struct {
	calls  [][]string
	script func(argv []string) (stdout, stderr string, err error)
}

func (f *fakeExecutor) Command(cmd string, args ...string) utilexec.Cmd {
	argv := append([]string{cmd}, args...)
	f.calls = append(f.calls, argv)

	var stdout, stderr string
	var err error
	if f.script != nil {
		stdout, stderr, err = f.script(argv)
	}

	return testingexec.InitFakeCmd(&testingexec.FakeCmd{
		OutputScript: []testingexec.FakeAction{
			func() ([]byte, []byte, error) { return []byte(stdout), []byte(stderr), err },
		},
		CombinedOutputScript: []testingexec.FakeAction{
			func() ([]byte, []byte, error) { return []byte(stdout + stderr), nil, err },
		},
	}, cmd, args...)
}

func (f *fakeExecutor) CommandContext(_ context.Context, cmd string, args ...string) utilexec.Cmd {
	return f.Command(cmd, args...)
}

const sessionTestIqn = "iqn.2000-01.com.synology:k8s-csi.pvc-aaa"

func sessionListLine(id int, iqn string) string {
	return fmt.Sprintf("tcp: [%d] 10.10.10.20:3260,1 %s (non-flash)\n", id, iqn)
}

// Guards the use of Output() over CombinedOutput(): a session-shaped line on
// stderr is synthetic, but nothing iscsiadm writes there may reach the parser.
func TestIscsiadmSessionParsesStdoutOnly(t *testing.T) {
	exec := &fakeExecutor{
		script: func(argv []string) (string, string, error) {
			return sessionListLine(1, sessionTestIqn),
				"iscsiadm: could not read session targetname: 5\n" + sessionListLine(2, "iqn.2000-01.com.synology:k8s-csi.pvc-stderr"),
				nil
		},
	}
	tools := NewTools(exec)

	got := tools.iscsiadm_session()

	want := []iscsiSession{
		{Protocol: "tcp", Id: 1, Portal: "10.10.10.20:3260", Iqn: sessionTestIqn, Name: "k8s-csi.pvc-aaa"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("iscsiadm_session() = %+v, want %+v", got, want)
	}
}
