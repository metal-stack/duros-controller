package controllers

const (
	lbCSIPluginImage            = "docker.lightbitslabs.com/lightos-csi/lb-csi-plugin:1.19.0"
	lbDiscoveryClientImage      = "docker.lightbitslabs.com/lightos-csi/lb-nvme-discovery-client:1.19.0"
	csiProvisionerImage         = "registry.k8s.io/sig-storage/csi-provisioner:v5.3.0"
	csiAttacherImage            = "registry.k8s.io/sig-storage/csi-attacher:v4.9.0"
	csiResizerImage             = "registry.k8s.io/sig-storage/csi-resizer:v1.14.0"
	csiNodeDriverRegistrarImage = "registry.k8s.io/sig-storage/csi-node-driver-registrar:v2.14.0"
	snapshotControllerImage     = "registry.k8s.io/sig-storage/snapshot-controller:v8.3.0"
	csiSnapshotterImage         = "registry.k8s.io/sig-storage/csi-snapshotter:v8.3.0"
)
