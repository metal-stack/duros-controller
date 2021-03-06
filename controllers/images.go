package controllers

const (
	lbCSIPluginImage            = "docker.lightbitslabs.com/lightos-csi/lb-csi-plugin:1.4.1"
	lbDiscoveryClientImage      = "docker.lightbitslabs.com/lightos-csi/lb-nvme-discovery-client:1.4.1"
	csiProvisionerImage         = "k8s.gcr.io/sig-storage/csi-provisioner:v1.6.1"
	csiAttacherImage            = "k8s.gcr.io/sig-storage/csi-attacher:v2.2.1"
	csiResizerImage             = "k8s.gcr.io/sig-storage/csi-resizer:v0.5.0"
	csiNodeDriverRegistrarImage = "k8s.gcr.io/sig-storage/csi-node-driver-registrar:v1.3.0"
	snapshotControllerImage     = "k8s.gcr.io/sig-storage/snapshot-controller:v4.1.0"
	csiSnapshotterImage         = "k8s.gcr.io/sig-storage/csi-snapshotter:v4.1.0"
	busyboxImage                = "busybox:1.32"
)
