package controllers

const (
	lbCSIPluginImage             = "docker.lightbitslabs.com/lightos-csi/lb-csi-plugin:1.9.1"
	lbDiscoveryClientImage       = "docker.lightbitslabs.com/lightos-csi/lb-nvme-discovery-client:1.9.1"
	csiProvisionerImage          = "registry.k8s.io/sig-storage/csi-provisioner:v2.2.2"
	csiAttacherImage             = "registry.k8s.io/sig-storage/csi-attacher:v3.5.0"
	csiResizerImage              = "registry.k8s.io/sig-storage/csi-resizer:v1.5.0"
	csiNodeDriverRegistrarImage  = "registry.k8s.io/sig-storage/csi-node-driver-registrar:v2.5.1"
	snapshotControllerImageBeta1 = "registry.k8s.io/sig-storage/snapshot-controller:v4.1.0"
	csiSnapshotterImageBeta1     = "registry.k8s.io/sig-storage/csi-snapshotter:v4.1.0"
	snapshotControllerImage      = "registry.k8s.io/sig-storage/snapshot-controller:v6.1.0" // for k8s >= 1.20
	csiSnapshotterImage          = "registry.k8s.io/sig-storage/csi-snapshotter:v6.1.0"     // for k8s >= 1.20
)
