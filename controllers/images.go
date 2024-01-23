package controllers

const (
	lbCSIPluginImage            = "docker.lightbitslabs.com/lightos-csi/lb-csi-plugin:1.14.0"
	lbDiscoveryClientImage      = "docker.lightbitslabs.com/lightos-csi/lb-nvme-discovery-client:1.14.0"
	csiProvisionerImage         = "registry.k8s.io/sig-storage/csi-provisioner:v3.6.3"
	csiAttacherImage            = "registry.k8s.io/sig-storage/csi-attacher:v4.4.3"
	csiResizerImage             = "registry.k8s.io/sig-storage/csi-resizer:v1.9.3"
	csiNodeDriverRegistrarImage = "registry.k8s.io/sig-storage/csi-node-driver-registrar:v2.9.3"
	snapshotControllerImage     = "registry.k8s.io/sig-storage/snapshot-controller:v6.3.3"
	csiSnapshotterImage         = "registry.k8s.io/sig-storage/csi-snapshotter:v6.3.3"
)
