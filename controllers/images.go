package controllers

const (
	lbCSIPluginImage            = "docker.lightbitslabs.com/lightos-csi/lb-csi-plugin:1.2.0"
	lbDiscoveryClientImage      = "docker.lightbitslabs.com/lightos-csi/lb-nvme-discovery-client:1.2.0"
	csiProvisionerImage         = "k8s.gcr.io/sig-storage/csi-provisioner:v1.5.0"
	csiAttacherImage            = "quay.io/k8scsi/csi-attacher:v2.1.0"
	csiResizerImage             = "k8s.gcr.io/sig-storage/csi-resizer:v0.5.0"
	csiNodeDriverRegistrarImage = "k8s.gcr.io/sig-storage/csi-node-driver-registrar:v1.2.0"
	busyboxImage                = "busybox:1.32"
)
