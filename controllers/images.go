package controllers

const (
	lbCSIPluginImage            = "docker.lightbitslabs.com/lightos-csi/lb-csi-plugin:1.17.0"
	lbDiscoveryClientImage      = "docker.lightbitslabs.com/lightos-csi/lb-nvme-discovery-client:1.17.0"
	csiProvisionerImage         = "registry.k8s.io/sig-storage/csi-provisioner:v5.2.0"
	csiAttacherImage            = "registry.k8s.io/sig-storage/csi-attacher:v4.8.0"
	csiResizerImage             = "registry.k8s.io/sig-storage/csi-resizer:v1.13.1"
	csiNodeDriverRegistrarImage = "registry.k8s.io/sig-storage/csi-node-driver-registrar:v2.13.0"
	snapshotControllerImage     = "registry.k8s.io/sig-storage/snapshot-controller:v8.2.0"
	csiSnapshotterImage         = "registry.k8s.io/sig-storage/csi-snapshotter:v8.2.0"
)
