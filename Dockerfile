# Copyright 2021 Synology Inc.

############## Build stage ##############
FROM --platform=$BUILDPLATFORM golang:1.21.4-alpine as builder
LABEL stage=synobuilder

RUN apk add --no-cache alpine-sdk
WORKDIR /go/src/synok8scsiplugin
COPY go.mod go.sum ./
RUN go mod download

COPY Makefile .

ARG TARGETPLATFORM

COPY main.go .
COPY pkg ./pkg
COPY synocli ./synocli
RUN env GOARCH=$(echo "$TARGETPLATFORM" | cut -f2 -d/) \
        GOARM=$(echo "$TARGETPLATFORM" | cut -f3 -d/ | cut -c2-) \
        make

############## Final stage ##############
FROM alpine:3.24
LABEL description="Synology CSI Plugin"

# Runtime tools. mkfs/fsck/blkid run inside this container (see
# pkg/driver/nodeserver.go NodeStageVolume), so every supported fsType needs
# its tools here. Upstream v1.4.0 moved to UBI 9 for OpenShift certification,
# which dropped btrfs-progs and needs RHEL entitlement to build; Alpine keeps
# fsType btrfs working and builds anywhere.
RUN apk add --no-cache \
        e2fsprogs e2fsprogs-extra xfsprogs xfsprogs-extra btrfs-progs \
        blkid util-linux iproute2 bash \
        ca-certificates cifs-utils nfs-utils nvme-cli

WORKDIR /

# Copy and run CSI driver
COPY --from=builder /go/src/synok8scsiplugin/bin/synology-csi-driver synology-csi-driver

# Declare a non-root user to satisfy the RunAsNonRoot certification check.
# The node DaemonSet still runs privileged with runAsUser: 0 (mount / iscsiadm
# need root); the controller Deployment can run as this user.
USER 1000

ENTRYPOINT ["/synology-csi-driver"]
