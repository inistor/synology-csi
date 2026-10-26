# Synology CSI Driver for Kubernetes

A [Container Storage Interface](https://github.com/container-storage-interface/spec) driver for Synology NAS.

This is an independently maintained **fork** of the official
[SynologyOpenSource/synology-csi](https://github.com/SynologyOpenSource/synology-csi), branched from its v1.4.0.
It is not affiliated with, endorsed by or supported by Synology Inc.; "Synology" and "DSM" are Synology's trademarks.

> **No warranty.** This software is provided "as is", without warranty of any kind, express or implied, as set out
> in sections 7 and 8 of the [Apache License 2.0](LICENSE). A storage driver can lose or corrupt data: test it
> against your own workloads and keep backups. You use it entirely at your own risk.

## How this fork differs from upstream

- **btrfs works.** Upstream v1.4.0 moved its image to Red Hat UBI 9, which dropped `btrfs-progs` and with it
  `fsType: btrfs`. This fork builds on Alpine and ships `btrfs-progs`, `e2fsprogs`, `xfsprogs`, NFS, CIFS and NVMe tools.
- **Reliability fixes**, several of them found in production:
  - a panic parsing `iscsiadm` output during concurrent teardown, which killed the node plugin mid-unstage;
    any panic in a request now fails only that request;
  - NodeUnpublishVolume reports *why* it could not remove a target path (still mounted vs. files left behind);
  - ReadWriteMany is refused for iSCSI/NVMe volumes with a filesystem, which several nodes mounting at once
    would corrupt; use NFS or SMB for ReadWriteMany;
  - DSM requests time out instead of hanging, reuse connections, and the driver exits when it can log in to no DSM;
  - volume stats come from the mount instead of a DSM listing per volume per minute;
  - NFS: an explicit client allowlist (`nfsClientAllowlist`) and `mountPermissions` setgid/sticky bits honoured.
- **A maintained Helm chart** as the single install path, for Kubernetes 1.35+, with current CSI sidecars,
  leader election and a non-root, unprivileged controller.
- **Current toolchain:** Go 1.27 and current releases of all dependencies.
- **Its own versions**, starting at 0.1.0 and unrelated to upstream's v1.x numbers.

Fixes are picked from upstream and other forks when they make sense, credited in the commit messages.

## Status

Under active reorganisation; there is no release yet. CI publishes an image for every push to
`ghcr.io/inistor/synology-csi`, tagged by branch and commit. Releases (`v0.1.0` onwards) will publish the image
with the version tag and the chart to `oci://ghcr.io/inistor/charts/synology-csi`.

## Features

| Protocol | Volume mode         | Access modes                     |
| -------- | ------------------- | -------------------------------- |
| iSCSI    | Filesystem, Block   | ReadWriteOnce (ReadWriteMany for Block only) |
| NVMe/TCP | Filesystem, Block   | ReadWriteOnce (ReadWriteMany for Block only) |
| NFS      | Filesystem          | ReadWriteOnce, ReadWriteMany     |
| SMB/CIFS | Filesystem          | ReadWriteOnce, ReadWriteMany     |

Plus dynamic provisioning, expansion, snapshots and cloning.
Driver name: `csi.san.synology.com`.

## Requirements

- Kubernetes 1.35 or later.
- DSM 7.0 or later (or DSM UC 3.1+, DSME 1.0+), with at least one storage pool and volume, and a dedicated DSM
  user for the driver.
- Every node can reach the DSM. Depending on the protocols you use, nodes need:
  - iSCSI: `open-iscsi` with `iscsid` running;
  - NVMe/TCP: the `nvme-tcp` kernel module;
  - btrfs/xfs volumes: kernel support for that filesystem.
- For snapshots: the [external-snapshotter](https://github.com/kubernetes-csi/external-snapshotter) CRDs and
  snapshot-controller (v8.x) installed in the cluster.

## Installation

The node plugin needs privileged pods, so its namespace must allow them:

```sh
kubectl create namespace synology-csi
kubectl label namespace synology-csi pod-security.kubernetes.io/enforce=privileged
```

Create the DSM connection Secret from a `client-info.yml` (see [Connecting to DSM](#connecting-to-dsm)):

```sh
cp config/client-info-template.yml config/client-info.yml   # then edit it
kubectl create secret generic client-info-secret -n synology-csi --from-file=config/client-info.yml
```

Alternatively, let the chart create it from `clientInfoSecret.clients` with `clientInfoSecret.create: true`.

Install the chart with your values (StorageClasses, snapshot classes, node settings; see
[deploy/helm/values.yaml](deploy/helm/values.yaml)):

```sh
# From a release:
helm install synology-csi oci://ghcr.io/inistor/charts/synology-csi --version <version> \
  --namespace synology-csi --values my-values.yaml

# Before the first release, from a checkout, with the image CI built from dev:
helm install synology-csi deploy/helm --namespace synology-csi --values my-values.yaml \
  --set images.plugin.tag=dev
```

Check that the controller and one node pod per node are running with `kubectl get pods -n synology-csi`, and run
`helm test synology-csi -n synology-csi` to provision and write a test volume.

### Upgrading from the upstream chart

The chart keeps upstream's resource names and the node DaemonSet's selector, so `helm upgrade` works on an
existing upstream release. Before upgrading:

- Remove any `images:` overrides from your values, or they will pin the old sidecars and upstream's image.
- The controller and snapshotter StatefulSets are replaced by a single controller Deployment.
- The controller no longer uses the host network: it must reach the DSM, and resolve its name, from the pod network.
- Snapshot RBAC changed with the newer snapshotter; make sure the external-snapshotter CRDs and controller are v8.x.

Uninstall with `helm uninstall`, but only once nothing uses volumes from this driver.

## Configuration

### Connecting to DSM

`client-info.yml` lists one or more DSMs:

```yaml
clients:
  - host: nas.example.com
    port: 5001
    https: true
    username: csi
    password: <password>
    tlsCACert: |
      -----BEGIN CERTIFICATE-----
      <DSM certificate or its CA, if not publicly trusted>
      -----END CERTIFICATE-----
```

- *host*: the DSM's hostname or IP address.
- *port*: 5000 for HTTP, 5001 for HTTPS by default.
- *https*: `true` to use HTTPS; set *port* to match.
- *username*, *password*: the DSM user the driver logs in as.
- *tlsCACert* (optional): PEM CA certificate for a DSM with a self-signed certificate. The system CA pool is always
  included, so publicly trusted certificates need nothing here.
- *tlsServerName* (optional): the name to verify the certificate against, e.g. when *host* is an IP address.
- *insecureSkipVerify* (optional): disables certificate verification. **Not recommended**: it exposes the DSM
  credentials to anyone in the middle. Use *tlsCACert* instead.

### StorageClasses

Define them in the chart's `storageClasses` value or apply your own; [deploy/example](deploy/example) has one per
protocol. Parameters, all optional:

| Name | Description | Default | Protocols |
| ---- | ----------- | ------- | --------- |
| *protocol* | `iscsi` or `nvme` for LUNs/namespaces, `nfs` or `smb` for shared folders | `iscsi` | all |
| *dsm* | Which DSM to use, as its *host* in `client-info.yml` | any | all |
| *location* | The DSM volume (`/volume1`, ...) to create volumes on | any with space | all |
| *fsType* | Filesystem to format the volume with: `ext4`, `xfs` or `btrfs` | `ext4` | iSCSI, NVMe |
| *formatOptions* | Extra arguments for `mkfs.<fsType>` | - | iSCSI, NVMe |
| *enableSpaceReclamation* | Space reclamation for thin-provisioned LUNs on Btrfs DSM volumes; may cost performance | `false` | iSCSI |
| *enableFuaSyncCache* | FUA and Sync Cache SCSI commands for the LUN | `false` | iSCSI |
| *mountPermissions* | Mode to `chmod` the mounted folder to; `0` skips the chmod | `0750` | NFS |
| *nfsClientAllowlist* | Comma-separated IPs, CIDRs, hostnames or `*` for the share's NFS rule, instead of every node's InternalIP; for when the DSM sees different source IPs (NAT, routed subnets) | node IPs | NFS |
| *csi.storage.k8s.io/node-stage-secret-name*, *-namespace* | Secret with the DSM `username`/`password` used to mount the share; **required** for SMB | - | SMB |

Notes:

- iSCSI LUNs are thin-provisioned, which is what allows snapshots.
- NFS and SMB versions and options go in the StorageClass `mountOptions`, e.g. `nfsvers=4.1`.
- Parameters cannot be changed on an existing StorageClass; create a new one instead.

### VolumeSnapshotClasses

Define them in the chart's `volumeSnapshotClasses` value. Parameters, all optional:

| Name | Description | Default | Protocols |
| ---- | ----------- | ------- | --------- |
| *description* | Description of the snapshot on DSM | `""` | iSCSI |
| *is_locked* | Whether to lock the snapshot on DSM | `false` | iSCSI, SMB, NFS |

## Development

- `make build`: the driver and the `synocli` tool, in `bin/`. `VERSION` defaults to `git describe`.
- `make test`: unit tests.
- `make docker-build`: a local image.
- `make sanity`: the CSI sanity suite against a real DSM. Needs root, `config/client-info.yml`, and
  `test/sanity/sanity-test-secret-file.yaml` copied from its template; `SANITY_PROTOCOL` picks the protocol
  (default `smb`).

CI checks formatting, builds, runs the unit tests and lints the chart on every push and pull request, and
publishes images for pushes and the chart for `v*` tags.

## License

[Apache License 2.0](LICENSE). Original work copyright Synology Inc.; their copyright notices are kept in the
files they wrote.
