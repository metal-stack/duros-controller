package controllers

const (
	lbCSIPluginImage            = "docker.lightbitslabs.com/lightos-csi/lb-csi-plugin:v1.21.0"
	lbDiscoveryClientImage      = "docker.lightbitslabs.com/lightos-csi/lb-nvme-discovery-client:v1.21.0"
	csiProvisionerImage         = "registry.k8s.io/sig-storage/csi-provisioner:v5.3.0"
	csiAttacherImage            = "registry.k8s.io/sig-storage/csi-attacher:v4.11.0"
	csiResizerImage             = "registry.k8s.io/sig-storage/csi-resizer:v2.1.0"
	csiNodeDriverRegistrarImage = "registry.k8s.io/sig-storage/csi-node-driver-registrar:v2.16.0"
	snapshotControllerImage     = "registry.k8s.io/sig-storage/snapshot-controller:v8.5.0"
	csiSnapshotterImage         = "registry.k8s.io/sig-storage/csi-snapshotter:v8.5.0"
)
