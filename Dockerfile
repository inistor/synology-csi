# Copyright 2021 Synology Inc.

############## Build stage ##############
# Cross-compiles on the build platform; the binary is static (CGO_ENABLED=0).
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS builder

RUN apk add --no-cache make
WORKDIR /go/src/synok8scsiplugin
COPY go.mod go.sum ./
RUN go mod download

COPY Makefile .

ARG TARGETPLATFORM
ARG VERSION=dev

COPY main.go .
COPY pkg ./pkg
COPY synocli ./synocli
RUN env GOARCH=$(echo "$TARGETPLATFORM" | cut -f2 -d/) \
        GOARM=$(echo "$TARGETPLATFORM" | cut -f3 -d/ | cut -c2-) \
        make VERSION="$VERSION"

############## Final stage ##############
FROM alpine:3.24
LABEL description="Synology CSI Plugin"

# Runtime tools. mkfs/fsck/blkid run inside this container (see
# pkg/driver/nodeserver.go NodeStageVolume), so every supported fsType needs
# its tools here, btrfs-progs included.
RUN apk add --no-cache \
        e2fsprogs e2fsprogs-extra xfsprogs xfsprogs-extra btrfs-progs \
        blkid util-linux iproute2 bash \
        ca-certificates cifs-utils nfs-utils nvme-cli

WORKDIR /

# Copy and run CSI driver
COPY --from=builder /go/src/synok8scsiplugin/bin/synology-csi-driver synology-csi-driver

# The controller runs as this user; the node plugin overrides it with root,
# which it needs to log in to targets, format and mount.
USER 65532

ENTRYPOINT ["/synology-csi-driver"]
