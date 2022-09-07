package controllers

const (
	lbCSIPluginImage            = "docker.lightbitslabs.com/lightos-csi-dev/lb-csi-plugin:pr18-129-49c50fb"
	lbDiscoveryClientImage      = "docker.lightbitslabs.com/lightos-csi/lb-nvme-discovery-client:1.9.0"
	csiProvisionerImage         = "k8s.gcr.io/sig-storage/csi-provisioner:v2.2.2"
	csiAttacherImage            = "k8s.gcr.io/sig-storage/csi-attacher:v3.5.0"
	csiResizerImage             = "k8s.gcr.io/sig-storage/csi-resizer:v1.5.0"
	csiNodeDriverRegistrarImage = "k8s.gcr.io/sig-storage/csi-node-driver-registrar:v2.5.1"
	snapshotControllerImage     = "k8s.gcr.io/sig-storage/snapshot-controller:v4.1.0"
	csiSnapshotterImage         = "k8s.gcr.io/sig-storage/csi-snapshotter:v4.1.0"
)
