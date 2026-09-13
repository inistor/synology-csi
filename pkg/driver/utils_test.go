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
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestLogGRPCRecoversFromPanic(t *testing.T) {
	info := &grpc.UnaryServerInfo{FullMethod: "/csi.v1.Node/NodeUnstageVolume"}
	handler := func(_ context.Context, _ interface{}) (interface{}, error) {
		panic("index out of range [1] with length 1")
	}

	resp, err := logGRPC(context.Background(), &csi.NodeUnstageVolumeRequest{VolumeId: "vol-1"}, info, handler)

	if err == nil {
		t.Fatalf("logGRPC() err = nil, want an error: a panicking handler must not take down the plugin")
	}
	if code := status.Code(err); code != codes.Internal {
		t.Errorf("logGRPC() code = %v, want %v", code, codes.Internal)
	}
	if resp != nil {
		t.Errorf("logGRPC() resp = %v, want nil", resp)
	}
}

func TestLogGRPCPassesThroughHandlerResult(t *testing.T) {
	info := &grpc.UnaryServerInfo{FullMethod: "/csi.v1.Node/NodeUnstageVolume"}
	want := &csi.NodeUnstageVolumeResponse{}
	handler := func(_ context.Context, _ interface{}) (interface{}, error) {
		return want, nil
	}

	resp, err := logGRPC(context.Background(), &csi.NodeUnstageVolumeRequest{VolumeId: "vol-1"}, info, handler)

	if err != nil {
		t.Fatalf("logGRPC() err = %v, want nil", err)
	}
	if resp != want {
		t.Errorf("logGRPC() resp = %v, want %v", resp, want)
	}
}
