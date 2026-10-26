# synology-csi Helm chart

Installs the [synology-csi](https://github.com/inistor/synology-csi) driver: a controller Deployment (provisioner,
attacher, resizer and snapshotter sidecars with leader election), a node DaemonSet, the `CSIDriver` object, and
the StorageClasses and VolumeSnapshotClasses you define in the values.

Requires Kubernetes 1.35+, a namespace that allows privileged pods, and a Secret with the DSM connection details
(or `clientInfoSecret.create: true`). Snapshots also need the external-snapshotter CRDs and snapshot-controller.

```sh
helm install synology-csi oci://ghcr.io/inistor/charts/synology-csi --version <version> \
  --namespace synology-csi --values my-values.yaml
```

All settings are documented in [values.yaml](values.yaml); installation, upgrading from the upstream chart and
the StorageClass parameters are in the [project README](https://github.com/inistor/synology-csi#readme).

The `Makefile` here wraps `helm upgrade --install`, `helm template`, `helm test` and `helm uninstall` for local use,
e.g. `make up OPTS="--values my-values.yaml"`.
