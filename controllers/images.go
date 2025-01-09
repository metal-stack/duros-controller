package controllers

const (
	lbCSIPluginImage            = "docker.lightbitslabs.com/lightos-csi/lb-csi-plugin:1.17.0"
	lbDiscoveryClientImage      = "docker.lightbitslabs.com/lightos-csi/lb-nvme-discovery-client:1.17.0"
	csiProvisionerImage         = "registry.k8s.io/sig-storage/csi-provisioner:v5.0.2"
	csiAttacherImage            = "registry.k8s.io/sig-storage/csi-attacher:v4.6.1"
	csiResizerImage             = "registry.k8s.io/sig-storage/csi-resizer:v1.11.2"
	csiNodeDriverRegistrarImage = "registry.k8s.io/sig-storage/csi-node-driver-registrar:v2.11.1"
	snapshotControllerImage     = "registry.k8s.io/sig-storage/snapshot-controller:v7.0.2"
	csiSnapshotterImage         = "registry.k8s.io/sig-storage/csi-snapshotter:v7.0.2"
)
