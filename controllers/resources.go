package controllers

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	"github.com/golang-jwt/jwt/v4"
	"github.com/metal-stack/duros-go"
	durosv2 "github.com/metal-stack/duros-go/api/duros/v2"
	metaltag "github.com/metal-stack/metal-lib/pkg/tag"

	storagev1 "github.com/metal-stack/duros-controller/api/v1"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1beta1"
	rbac "k8s.io/api/rbac/v1"
	storage "k8s.io/api/storage/v1"
	storagev1beta1 "k8s.io/api/storage/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	namespace   = "kube-system"
	provisioner = "csi.lightbitslabs.com"

	// nolint: gosec
	storageClassCredentialsRef = "lb-csi-creds"

	lbCSIControllerName = "lb-csi-controller"
	lbCSINodeName       = "lb-csi-node"

	tokenLifetime      = 8 * 24 * time.Hour
	tokenRenewalBefore = 1 * 24 * time.Hour
)

var (
	hostPathDirectoryOrCreate       = corev1.HostPathDirectoryOrCreate
	hostPathDirectory               = corev1.HostPathDirectory
	mountPropagationHostToContainer = corev1.MountPropagationHostToContainer
	mountPropagationBidirectional   = corev1.MountPropagationBidirectional

	// PSP
	pspController = policy.PodSecurityPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "lb-csi-ctrl-sa",
		},
		Spec: policy.PodSecurityPolicySpec{
			FSGroup:            policy.FSGroupStrategyOptions{Rule: policy.FSGroupStrategyRunAsAny},
			RunAsUser:          policy.RunAsUserStrategyOptions{Rule: policy.RunAsUserStrategyRunAsAny},
			SELinux:            policy.SELinuxStrategyOptions{Rule: policy.SELinuxStrategyRunAsAny},
			SupplementalGroups: policy.SupplementalGroupsStrategyOptions{Rule: policy.SupplementalGroupsStrategyRunAsAny},
			Volumes: []policy.FSType{
				"secret",
				"emptyDir",
			},
		},
	}
	pspNode = policy.PodSecurityPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "lb-csi-node-sa",
		},
		Spec: policy.PodSecurityPolicySpec{
			AllowedCapabilities: []corev1.Capability{"SYS_ADMIN"},
			AllowedHostPaths: []policy.AllowedHostPath{
				{PathPrefix: "/var/lib/kubelet"},
				{PathPrefix: "/dev"},
				{PathPrefix: "/lib/modules"},
				{PathPrefix: "/var/lib/kubelet/*"},
			},
			HostNetwork:        true,
			Privileged:         true,
			FSGroup:            policy.FSGroupStrategyOptions{Rule: policy.FSGroupStrategyRunAsAny},
			RunAsUser:          policy.RunAsUserStrategyOptions{Rule: policy.RunAsUserStrategyRunAsAny},
			SELinux:            policy.SELinuxStrategyOptions{Rule: policy.SELinuxStrategyRunAsAny},
			SupplementalGroups: policy.SupplementalGroupsStrategyOptions{Rule: policy.SupplementalGroupsStrategyRunAsAny},
			Volumes: []policy.FSType{
				"secret",
				"emptyDir",
				"hostPath",
			},
		},
	}

	psps = []policy.PodSecurityPolicy{
		pspNode,
		pspController,
	}

	// ServiceAccounts
	ctrlServiceAccount = corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "lb-csi-ctrl-sa",
			Namespace: namespace,
		},
	}

	nodeServiceAccount = corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "lb-csi-node-sa",
			Namespace: namespace,
		},
	}
	serviceAccounts = []corev1.ServiceAccount{ctrlServiceAccount, nodeServiceAccount}

	// ClusterRoles
	nodeClusterRole = rbac.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "lb-csi-node",
		},
		Rules: []rbac.PolicyRule{
			{
				APIGroups:     []string{"policy"},
				Resources:     []string{"podsecuritypolicies"},
				Verbs:         []string{"use"},
				ResourceNames: []string{nodeServiceAccount.Name},
			},
		},
	}
	nodeClusterRoleBinding = rbac.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "lb-csi-node",
		},
		Subjects: []rbac.Subject{
			{
				Name:      "lb-csi-node-sa",
				Kind:      "ServiceAccount",
				Namespace: namespace,
			},
		},
		RoleRef: rbac.RoleRef{
			Name:     "lb-csi-node",
			Kind:     "ClusterRole",
			APIGroup: "rbac.authorization.k8s.io",
		},
	}

	ctrlClusterRole = rbac.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "lb-csi-provisioner-role",
		},
		Rules: []rbac.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"secrets"},
				Verbs:     []string{"get", "list"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"persistentvolumes"},
				Verbs:     []string{"get", "list", "watch", "create", "delete"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"persistentvolumeclaims"},
				Verbs:     []string{"get", "list", "watch", "update"},
			},
			{
				APIGroups: []string{"storage.k8s.io"},
				Resources: []string{"storageclasses"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{"storage.k8s.io"},
				Resources: []string{"csinodes"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"events"},
				Verbs:     []string{"list", "watch", "create", "update", "patch"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"nodes"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups:     []string{"policy"},
				Resources:     []string{"podsecuritypolicies"},
				Verbs:         []string{"use"},
				ResourceNames: []string{ctrlServiceAccount.Name},
			},
		},
	}

	attacherClusterRole = rbac.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "lb-csi-attacher-role",
		},
		Rules: []rbac.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"persistentvolumes"},
				Verbs:     []string{"get", "list", "watch", "create", "delete", "patch"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"nodes"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{"storage.k8s.io"},
				Resources: []string{"csinodes"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{"storage.k8s.io"},
				Resources: []string{"volumeattachments", "volumeattachments/status"},
				Verbs:     []string{"get", "list", "watch", "update", "patch"},
			},
			{
				APIGroups:     []string{"policy"},
				Resources:     []string{"podsecuritypolicies"},
				Verbs:         []string{"use"},
				ResourceNames: []string{ctrlServiceAccount.Name},
			},
		},
	}

	resizerClusterRole = rbac.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "external-resizer-runner",
		},
		Rules: []rbac.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"persistentvolumes"},
				Verbs:     []string{"get", "list", "watch", "create", "delete"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"persistentvolumeclaims"},
				Verbs:     []string{"get", "list", "watch", "update"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"persistentvolumeclaims/status"},
				Verbs:     []string{"patch"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"events"},
				Verbs:     []string{"list", "watch", "create", "update", "patch"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups:     []string{"policy"},
				Resources:     []string{"podsecuritypolicies"},
				Verbs:         []string{"use"},
				ResourceNames: []string{ctrlServiceAccount.Name},
			},
		},
	}

	snapshotClusterRole = rbac.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "snapshot-controller-runner",
		},
		Rules: []rbac.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"persistentvolumes"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"persistentvolumeclaims"},
				Verbs:     []string{"get", "list", "watch", "update"},
			},
			{
				APIGroups: []string{"storage.k8s.io"},
				Resources: []string{"storageclasses"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"events"},
				Verbs:     []string{"list", "watch", "create", "update", "patch"},
			},
			{
				APIGroups: []string{"snapshot.storage.k8s.io"},
				Resources: []string{"volumesnapshotclasses"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{"snapshot.storage.k8s.io"},
				Resources: []string{"volumesnapshotcontents"},
				Verbs:     []string{"create", "get", "list", "watch", "update", "delete"},
			},
			{
				APIGroups: []string{"snapshot.storage.k8s.io"},
				Resources: []string{"volumesnapshots"},
				Verbs:     []string{"get", "list", "watch", "update"},
			},
			{
				APIGroups: []string{"snapshot.storage.k8s.io"},
				Resources: []string{"volumesnapshots/status"},
				Verbs:     []string{"update"},
			},
		},
	}
	externalSnapshotterClusterRole = rbac.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "external-snapshotter-runner",
		},
		Rules: []rbac.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"events"},
				Verbs:     []string{"list", "watch", "create", "update", "patch"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"secrets"},
				Verbs:     []string{"get", "list"},
			},
			{
				APIGroups: []string{"snapshot.storage.k8s.io"},
				Resources: []string{"volumesnapshotclasses"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{"snapshot.storage.k8s.io"},
				Resources: []string{"volumesnapshotcontents"},
				Verbs:     []string{"create", "get", "list", "watch", "update", "delete"},
			},
			{
				APIGroups: []string{"snapshot.storage.k8s.io"},
				Resources: []string{"volumesnapshotcontents/status"},
				Verbs:     []string{"update"},
			},
		},
	}

	clusterRoles = []rbac.ClusterRole{
		nodeClusterRole,
		attacherClusterRole,
		ctrlClusterRole,
		resizerClusterRole,
		snapshotClusterRole,
		externalSnapshotterClusterRole,
	}

	ctrlClusterRoleBinding = rbac.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "lb-csi-provisioner-binding",
		},
		Subjects: []rbac.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      ctrlServiceAccount.Name,
				Namespace: ctrlServiceAccount.Namespace,
			},
		},
		RoleRef: rbac.RoleRef{
			Kind:     "ClusterRole",
			Name:     ctrlClusterRole.Name,
			APIGroup: ctrlClusterRole.APIVersion,
		},
	}

	attacherClusterRoleBinding = rbac.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "lb-csi-attacher-binding",
		},
		Subjects: []rbac.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      ctrlServiceAccount.Name,
				Namespace: ctrlServiceAccount.Namespace,
			},
		},
		RoleRef: rbac.RoleRef{
			Kind:     "ClusterRole",
			Name:     attacherClusterRole.Name,
			APIGroup: attacherClusterRole.APIVersion,
		},
	}

	resizerClusterRoleBinding = rbac.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "csi-resizer-role",
		},
		Subjects: []rbac.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      ctrlServiceAccount.Name,
				Namespace: ctrlServiceAccount.Namespace,
			},
		},
		RoleRef: rbac.RoleRef{
			Kind:     "ClusterRole",
			Name:     resizerClusterRole.Name,
			APIGroup: resizerClusterRole.APIVersion,
		},
	}
	snapshotClusterRoleBinding = rbac.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "snapshot-controller-role",
		},
		Subjects: []rbac.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      ctrlServiceAccount.Name,
				Namespace: ctrlServiceAccount.Namespace,
			},
		},
		RoleRef: rbac.RoleRef{
			Kind:     "ClusterRole",
			Name:     snapshotClusterRole.Name,
			APIGroup: snapshotClusterRole.APIVersion,
		},
	}
	externalSnapshotterClusterRoleBinding = rbac.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "csi-snapshotter-role",
		},
		Subjects: []rbac.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      ctrlServiceAccount.Name,
				Namespace: ctrlServiceAccount.Namespace,
			},
		},
		RoleRef: rbac.RoleRef{
			Kind:     "ClusterRole",
			Name:     externalSnapshotterClusterRole.Name,
			APIGroup: externalSnapshotterClusterRole.APIVersion,
		},
	}
	clusterRoleBindings = []rbac.ClusterRoleBinding{
		nodeClusterRoleBinding,
		attacherClusterRoleBinding,
		ctrlClusterRoleBinding,
		resizerClusterRoleBinding,
		snapshotClusterRoleBinding,
		externalSnapshotterClusterRoleBinding,
	}

	// ResourceLimits
	cpu100m, _            = resource.ParseQuantity("100m")
	memory100m, _         = resource.ParseQuantity("100M")
	cpu200m, _            = resource.ParseQuantity("200m")
	memory200m, _         = resource.ParseQuantity("200M")
	defaultResourceLimits = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			"cpu":    cpu100m,
			"memory": memory100m,
		},
		Limits: corev1.ResourceList{
			"cpu":    cpu200m,
			"memory": memory200m,
		},
	}

	// Containers
	csiPluginContainer = corev1.Container{
		Name:            "lb-csi-plugin",
		Image:           lbCSIPluginImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Args:            []string{"-P"},
		Env: []corev1.EnvVar{
			{Name: "CSI_ENDPOINT", Value: "unix:///var/lib/csi/sockets/pluginproxy/csi.sock"},
			{Name: "KUBE_NODE_NAME", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "spec.nodeName"}}},
			{Name: "LB_CSI_NODE_ID", Value: "$(KUBE_NODE_NAME).ctrl"},
			{Name: "LB_CSI_LOG_LEVEL", Value: "debug"},
			{Name: "LB_CSI_LOG_ROLE", Value: "controller"},
			{Name: "LB_CSI_LOG_FMT", Value: "text"},
			{Name: "LB_CSI_LOG_TIME", Value: "true"},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: socketDirVolume.Name, MountPath: "/var/lib/csi/sockets/pluginproxy/"},
			{Name: etcDirVolume.Name, MountPath: "/etc/lb-csi/"},
		},
		Resources: defaultResourceLimits,
	}
	csiProvisionerContainer = corev1.Container{
		Name:            "csi-provisioner",
		Image:           csiProvisionerImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Args:            []string{"--csi-address=$(ADDRESS)", "--v=4"},
		Env: []corev1.EnvVar{
			{Name: "ADDRESS", Value: "/var/lib/csi/sockets/pluginproxy/csi.sock"},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: socketDirVolume.Name, MountPath: "/var/lib/csi/sockets/pluginproxy/"},
		},
		Resources: defaultResourceLimits,
	}
	csiAttacherContainer = corev1.Container{
		Name:            "csi-attacher",
		Image:           csiAttacherImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Args:            []string{"--csi-address=$(ADDRESS)", "--v=5"},
		Env: []corev1.EnvVar{
			{Name: "ADDRESS", Value: "/var/lib/csi/sockets/pluginproxy/csi.sock"},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: socketDirVolume.Name, MountPath: "/var/lib/csi/sockets/pluginproxy/"},
		},
		Resources: defaultResourceLimits,
	}
	csiResizerContainer = corev1.Container{
		Name:            "csi-resizer",
		Image:           csiResizerImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Args:            []string{"--csi-address=$(ADDRESS)", "--v=4"},
		Env: []corev1.EnvVar{
			{Name: "ADDRESS", Value: "/var/lib/csi/sockets/pluginproxy/csi.sock"},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: socketDirVolume.Name, MountPath: "/var/lib/csi/sockets/pluginproxy/"},
		},
		Resources: defaultResourceLimits,
	}
	snapshotControllerContainer = corev1.Container{
		Name:            "snapshot-controller",
		Image:           snapshotControllerImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Args:            []string{"--leader-election=false", "--v=5"},
		Resources:       defaultResourceLimits,
	}
	csiSnapshotterContainer = corev1.Container{
		Name:            "csi-snapshotter",
		Image:           csiSnapshotterImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Args:            []string{"--csi-address=$(ADDRESS)", "--leader-election=false", "--v=5"},
		Env: []corev1.EnvVar{
			{Name: "ADDRESS", Value: "/var/lib/csi/sockets/pluginproxy/csi.sock"},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: socketDirVolume.Name, MountPath: "/var/lib/csi/sockets/pluginproxy/"},
		},
		Resources: defaultResourceLimits,
	}
	discoveryClientContainer = corev1.Container{
		Name:            "lb-nvme-discovery-client",
		Image:           lbDiscoveryClientImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		VolumeMounts: []corev1.VolumeMount{
			{Name: deviceDirVolume.Name, MountPath: "/dev"},
			{Name: discoveryClientDirVolume.Name, MountPath: "/etc/discovery-client/discovery.d"},
		},
		SecurityContext: &corev1.SecurityContext{
			Privileged: pointer.Bool(true),
			Capabilities: &corev1.Capabilities{
				Add: []corev1.Capability{"SYS_ADMIN"},
			},
			AllowPrivilegeEscalation: pointer.Bool(true),
		},
		Resources: defaultResourceLimits,
	}

	nodeInitContainer = corev1.Container{
		Name:            "init-nvme-tcp",
		Image:           busyboxImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		SecurityContext: &corev1.SecurityContext{Privileged: pointer.Bool(true)},
		VolumeMounts: []corev1.VolumeMount{
			{Name: modulesDirVolume.Name, MountPath: "/lib/modules", MountPropagation: &mountPropagationHostToContainer},
		},
		Command: []string{
			"/bin/sh",
			"-c",
			`[ -e /sys/module/nvme_tcp ] && modinfo nvme_tcp || { modinfo nvme_tcp && modprobe nvme_tcp ; } || { echo \"FAILED to load nvme-tcp kernel driver\" && exit 1 ; }`,
		},
		Resources: defaultResourceLimits,
	}

	csiPluginNodeContainer = corev1.Container{
		Name:            "lb-csi-plugin",
		Image:           lbCSIPluginImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		SecurityContext: &corev1.SecurityContext{
			Privileged:               pointer.Bool(true),
			AllowPrivilegeEscalation: pointer.Bool(true),
			Capabilities:             &corev1.Capabilities{Add: []corev1.Capability{"SYS_ADMIN"}},
		},
		Args: []string{"-P"},
		Env: []corev1.EnvVar{
			{Name: "CSI_ENDPOINT", Value: "unix:///csi/csi.sock"},
			{Name: "KUBE_NODE_NAME", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "spec.nodeName"}}},
			{Name: "LB_CSI_NODE_ID", Value: "$(KUBE_NODE_NAME).node"},
			{Name: "LB_CSI_LOG_LEVEL", Value: "debug"},
			{Name: "LB_CSI_LOG_ROLE", Value: "node"},
			{Name: "LB_CSI_LOG_FMT", Value: "text"},
			{Name: "LB_CSI_LOG_TIME", Value: "true"},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: pluginDirVolume.Name, MountPath: "/csi"},
			{Name: podsMountDirVolume.Name, MountPath: "/var/lib/kubelet", MountPropagation: &mountPropagationBidirectional},
			{Name: deviceDirVolume.Name, MountPath: "/dev"},
			{Name: discoveryClientDirVolume.Name, MountPath: "/etc/discovery-client/discovery.d"},
			{Name: etcDirVolume.Name, MountPath: "/etc/lb-csi/"},
		},
		Resources: defaultResourceLimits,
	}

	csiNodeDriverRegistrarContainer = corev1.Container{
		Name:            "csi-node-driver-registrar",
		Image:           csiNodeDriverRegistrarImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Args: []string{
			"--v=4",
			"--csi-address=$(ADDRESS)",
			"--kubelet-registration-path=$(DRIVER_REG_SOCK_PATH)",
		},
		Env: []corev1.EnvVar{
			{Name: "ADDRESS", Value: "/csi/csi.sock"},
			{Name: "DRIVER_REG_SOCK_PATH", Value: "/var/lib/kubelet/plugins/csi.lightbitslabs.com/csi.sock"},
			{Name: "KUBE_NODE_NAME", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "spec.nodeName"}}},
		},
		Lifecycle: &corev1.Lifecycle{PreStop: &corev1.LifecycleHandler{Exec: &corev1.ExecAction{
			Command: []string{
				"/bin/sh",
				"-c",
				"rm -rf /registration/csi.lightbitslabs.com /registration/csi.lightbitslabs.com-reg.sock",
			},
		}}},
		VolumeMounts: []corev1.VolumeMount{
			{Name: pluginDirVolume.Name, MountPath: "/csi"},
			{Name: registrationDirVolume.Name, MountPath: "/registration/"},
		},
		Resources: defaultResourceLimits,
	}

	// Volumes
	socketDirVolume = corev1.Volume{
		Name:         "socket-dir",
		VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
	}
	discoveryClientDirVolume = corev1.Volume{
		Name:         "discovery-client-dir",
		VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
	}
	registrationDirVolume = corev1.Volume{
		Name: "registration-dir",
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: "/var/lib/kubelet/plugins_registry/",
				Type: &hostPathDirectoryOrCreate,
			},
		},
	}
	pluginDirVolume = corev1.Volume{
		Name: "plugin-dir",
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: "/var/lib/kubelet/plugins/csi.lightbitslabs.com",
				Type: &hostPathDirectoryOrCreate,
			},
		},
	}
	podsMountDirVolume = corev1.Volume{
		Name: "pods-mount-dir",
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: "/var/lib/kubelet",
				Type: &hostPathDirectory,
			},
		},
	}
	deviceDirVolume = corev1.Volume{
		Name: "device-dir",
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: "/dev",
			},
		},
	}
	modulesDirVolume = corev1.Volume{
		Name: "modules-dir",
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: "/lib/modules",
			},
		},
	}
	etcDirVolume = corev1.Volume{
		Name: "etc-lb-csi",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: storageClassCredentialsRef,
			},
		},
	}

	// Node DaemonSet
	nodeRoleLabels   = map[string]string{"app": lbCSINodeName, "role": "node"}
	csiNodeDaemonSet = apps.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      lbCSINodeName,
			Namespace: namespace,
			Labels:    map[string]string{"shoot.gardener.cloud/no-cleanup": "true"},
		},
		Spec: apps.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: nodeRoleLabels},
			UpdateStrategy: apps.DaemonSetUpdateStrategy{
				Type:          apps.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &apps.RollingUpdateDaemonSet{MaxUnavailable: &intstr.IntOrString{IntVal: 1}},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: nodeRoleLabels},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						nodeInitContainer,
					},
					Containers: []corev1.Container{
						csiPluginNodeContainer,
						csiNodeDriverRegistrarContainer,
						discoveryClientContainer,
					},
					ServiceAccountName: nodeServiceAccount.Name,
					PriorityClassName:  "system-node-critical",
					HostNetwork:        true,
					Volumes: []corev1.Volume{
						registrationDirVolume,
						pluginDirVolume,
						podsMountDirVolume,
						deviceDirVolume,
						modulesDirVolume,
						discoveryClientDirVolume,
						etcDirVolume,
					},
				},
			},
		},
	}
)

func (r *DurosReconciler) reconcileStorageClassSecret(ctx context.Context, credential *durosv2.Credential, adminKey []byte) error {
	var (
		log    = r.Log.WithName("storage-class")
		secret = &corev1.Secret{}
	)

	key := types.NamespacedName{Name: storageClassCredentialsRef, Namespace: namespace}
	err := r.Shoot.Get(ctx, key, secret)
	if err != nil && apierrors.IsNotFound(err) {
		log.Info("deploy storage-class-secret")
		return r.deployStorageClassSecret(ctx, log, credential, adminKey)
	}
	if err != nil {
		return fmt.Errorf("unable to read secret: %w", err)
	}

	// secret already exists, check for renewal
	token, ok := secret.Data["jwt"]
	if !ok {
		log.Error(fmt.Errorf("no storage class token present in existing token"), "recreating storage-class-secret")
		err := r.deleteResourceWithWait(ctx, log, deletionResource{
			Key:    key,
			Object: secret,
		})
		if err != nil {
			return err
		}
		return r.deployStorageClassSecret(ctx, log, credential, adminKey)
	}

	claims := &jwt.RegisteredClaims{}
	_, _, err = new(jwt.Parser).ParseUnverified(string(token), claims)
	if err != nil {
		log.Error(err, "storage class token not parsable, recreating storage-class-secret")
		err := r.deleteResourceWithWait(ctx, log, deletionResource{
			Key:    key,
			Object: secret,
		})
		if err != nil {
			return err
		}
		return r.deployStorageClassSecret(ctx, log, credential, adminKey)
	}

	renewalAt := claims.ExpiresAt.Add(-tokenRenewalBefore)
	if time.Now().After(renewalAt) {
		log.Info("storage class token is expiring soon, refreshing token", "expires-at", claims.ExpiresAt.String())
		return r.deployStorageClassSecret(ctx, log, credential, adminKey)
	}

	log.Info("storage class token is not expiring soon, not doing anything", "expires-at", claims.ExpiresAt.String(), "renewal-at", renewalAt.String())

	return nil
}

func (r *DurosReconciler) deployStorageClassSecret(ctx context.Context, log logr.Logger, credential *durosv2.Credential, adminKey []byte) error {
	key, err := extract(adminKey)
	if err != nil {
		return err
	}

	token, err := duros.NewJWTTokenForCredential(r.Namespace, "duros-controller", credential, []string{credential.ProjectName + ":admin"}, tokenLifetime, key)
	if err != nil {
		return fmt.Errorf("unable to create jwt token:%w", err)
	}

	storageClassSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: storageClassCredentialsRef, Namespace: namespace},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Shoot, &storageClassSecret, func() error {
		storageClassSecret.Type = "kubernetes.io/lb-csi"
		storageClassSecret.Data = map[string][]byte{
			"jwt": []byte(token),
		}

		return nil
	})
	if err != nil {
		return err
	}

	log.Info("storageclasssecret", "name", storageClassCredentialsRef, "operation", op)

	return nil
}

func (r *DurosReconciler) deployCSI(ctx context.Context, projectID string, scs []storagev1.StorageClass) error {
	log := r.Log.WithName("storage-csi")
	log.Info("deploy storage-class")

	rm := r.Shoot.RESTMapper()
	gkv, err := rm.ResourceFor(schema.GroupVersionResource{
		Group:    "storage.k8s.io",
		Resource: "CSIDriver",
	})
	if err != nil {
		return err
	}
	log.Info("sc supported", "group", gkv.Group, "kind", gkv.Resource, "version", gkv.Version)

	snapshotsSupported := false
	switch gkv.Version {
	case "v1":
		csiDriver := &storage.CSIDriver{ObjectMeta: metav1.ObjectMeta{Name: provisioner}}
		op, err := controllerutil.CreateOrUpdate(ctx, r.Shoot, csiDriver, func() error {
			csiDriver.Spec = storage.CSIDriverSpec{
				AttachRequired: pointer.Bool(true),
				PodInfoOnMount: pointer.Bool(true),
			}
			return nil
		})
		if err != nil {
			return err
		}
		log.Info("csidriver", "name", csiDriver.Name, "operation", op)
		snapshotsSupported = true
	case "v1beta1":
		csiDriver := &storagev1beta1.CSIDriver{ObjectMeta: metav1.ObjectMeta{Name: provisioner}}
		op, err := controllerutil.CreateOrUpdate(ctx, r.Shoot, csiDriver, func() error {
			csiDriver.Spec = storagev1beta1.CSIDriverSpec{
				AttachRequired: pointer.Bool(true),
				PodInfoOnMount: pointer.Bool(true),
			}
			return nil
		})
		if err != nil {
			return err
		}
		log.Info("csidriver", "name", csiDriver.Name, "operation", op)
	default:
		err := fmt.Errorf("unsupported csi driver version:%s", gkv.Version)
		log.Error(err, "no csi plugin deployment possible")
		return err
	}

	for i := range psps {
		psp := psps[i]
		obj := &policy.PodSecurityPolicy{ObjectMeta: metav1.ObjectMeta{Name: psp.Name}}
		op, err := controllerutil.CreateOrUpdate(ctx, r.Shoot, obj, func() error {
			obj.Spec = psp.Spec
			return nil
		})
		if err != nil {
			return err
		}
		log.Info("psp", "name", psp.Name, "operation", op)
	}

	for i := range serviceAccounts {
		sa := serviceAccounts[i]
		obj := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: sa.Name, Namespace: sa.Namespace}}
		op, err := controllerutil.CreateOrUpdate(ctx, r.Shoot, obj, func() error {
			return nil
		})
		if err != nil {
			return err
		}
		log.Info("serviceaccount", "name", sa.Name, "operation", op)
	}

	for i := range clusterRoles {
		cr := clusterRoles[i]
		obj := &rbac.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: cr.Name, Namespace: cr.Namespace}}
		op, err := controllerutil.CreateOrUpdate(ctx, r.Shoot, obj, func() error {
			obj.Rules = cr.Rules
			return nil
		})
		if err != nil {
			return err
		}
		log.Info("clusterrole", "name", cr.Name, "operation", op)
	}

	for i := range clusterRoleBindings {
		crb := clusterRoleBindings[i]
		obj := &rbac.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: crb.Name, Namespace: crb.Namespace}}
		op, err := controllerutil.CreateOrUpdate(ctx, r.Shoot, obj, func() error {
			obj.Subjects = crb.Subjects
			obj.RoleRef = crb.RoleRef
			return nil
		})
		if err != nil {
			return err
		}
		log.Info("clusterrolebindinding", "name", crb.Name, "operation", op)
	}

	sts := &apps.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      lbCSIControllerName,
			Namespace: namespace,
			Labels:    map[string]string{"shoot.gardener.cloud/no-cleanup": "true"},
		},
	}
	op, err := controllerutil.CreateOrUpdate(ctx, r.Shoot, sts, func() error {

		controllerRoleLabels := map[string]string{"app": "lb-csi-plugin", "role": "controller"}
		containers := []corev1.Container{
			csiPluginContainer,
			csiProvisionerContainer,
			csiAttacherContainer,
			csiResizerContainer,
		}
		if snapshotsSupported {
			containers = append(containers, snapshotControllerContainer, csiSnapshotterContainer)
		}

		sts.Spec = apps.StatefulSetSpec{
			Selector:    &metav1.LabelSelector{MatchLabels: controllerRoleLabels},
			ServiceName: "lb-csi-ctrl-svc",
			Replicas:    pointer.Int32(1),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: controllerRoleLabels},
				Spec: corev1.PodSpec{
					Containers:         containers,
					ServiceAccountName: ctrlServiceAccount.Name,
					PriorityClassName:  "system-cluster-critical",
					Volumes: []corev1.Volume{
						socketDirVolume,
						etcDirVolume,
					},
				},
			},
		}
		return nil
	})

	if err != nil {
		return err
	}
	log.Info("statefulset", "name", sts.Name, "operation", op)

	ds := &apps.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: csiNodeDaemonSet.Name, Namespace: csiNodeDaemonSet.Namespace}}
	op, err = controllerutil.CreateOrUpdate(ctx, r.Shoot, ds, func() error {
		ds.Spec = csiNodeDaemonSet.Spec
		return nil
	})
	if err != nil {
		return err
	}
	log.Info("daemonset", "name", csiNodeDaemonSet.Name, "operation", op)

	for i := range scs {
		sc := scs[i]

		annotations := map[string]string{
			"storageclass.kubernetes.io/is-default-class": strconv.FormatBool(sc.Default),
			metaltag.ClusterDescription:                   "DO NOT EDIT - This resource is managed by duros-controller. Any modifications are discarded and the resource is returned to the original state.",
		}

		obj := &storage.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: sc.Name}}
		op, err = controllerutil.CreateOrUpdate(ctx, r.Shoot, obj, func() error {
			obj.ObjectMeta.Annotations = annotations
			obj.Provisioner = provisioner
			obj.AllowVolumeExpansion = pointer.Bool(true)
			obj.Parameters = map[string]string{
				"mgmt-scheme":   "grpcs",
				"compression":   "disabled",
				"mgmt-endpoint": r.Endpoints.String(),
				"project-name":  projectID,
				"replica-count": strconv.Itoa(sc.ReplicaCount),
				"csi.storage.k8s.io/fstype": "ext4",
				"csi.storage.k8s.io/controller-expand-secret-name":       storageClassCredentialsRef,
				"csi.storage.k8s.io/controller-expand-secret-namespace":  namespace,
				"csi.storage.k8s.io/controller-publish-secret-name":      storageClassCredentialsRef,
				"csi.storage.k8s.io/controller-publish-secret-namespace": namespace,
				"csi.storage.k8s.io/node-publish-secret-name":            storageClassCredentialsRef,
				"csi.storage.k8s.io/node-publish-secret-namespace":       namespace,
				"csi.storage.k8s.io/node-stage-secret-name":              storageClassCredentialsRef,
				"csi.storage.k8s.io/node-stage-secret-namespace":         namespace,
				"csi.storage.k8s.io/provisioner-secret-name":             storageClassCredentialsRef,
				"csi.storage.k8s.io/provisioner-secret-namespace":        namespace,
			}

			if sc.Compression {
				obj.Parameters["compression"] = "enabled"
			}
			// if sc.Encryption {
			// 	secretName := "storage-encryption-key"
			// 	//nolint:gosec
			// 	secretNamespace := "${pvc.namespace}"
			// 	obj.Parameters["encryption"] = "enabled"
			// 	obj.Parameters["csi.storage.k8s.io/controller-expand-secret-name"] = secretName
			// 	obj.Parameters["csi.storage.k8s.io/controller-expand-secret-namespace"] = secretNamespace
			// 	obj.Parameters["csi.storage.k8s.io/node-publish-secret-name"] = secretName
			// 	obj.Parameters["csi.storage.k8s.io/node-publish-secret-namespace"] = secretNamespace
			// 	obj.Parameters["csi.storage.k8s.io/node-stage-secret-name"] = secretName
			// 	obj.Parameters["csi.storage.k8s.io/node-stage-secret-namespace"] = secretNamespace
			// }
			return nil
		})
		if err != nil {
			return err
		}
		log.Info("storageclass", "name", sc.Name, "operation", op)
	}

	return nil
}

type deletionResource struct {
	Key    types.NamespacedName
	Object client.Object
}

func (r *DurosReconciler) deleteResourceWithWait(ctx context.Context, log logr.Logger, resource deletionResource) error {
	err := r.Shoot.Get(ctx, resource.Key, resource.Object)
	if err != nil && apierrors.IsNotFound(err) {
		// already deleted
		return nil
	}
	if err != nil {
		return fmt.Errorf("error getting resource during deletion flow: %w", err)
	}

	log.Info("cleaning up resource", "name", resource.Key.Name, "namespace", resource.Key.Namespace)
	return wait.PollImmediateInfiniteWithContext(ctx, 100*time.Millisecond, func(context.Context) (done bool, err error) {
		err = r.Shoot.Delete(ctx, resource.Object)

		if apierrors.IsNotFound(err) || apierrors.IsConflict(err) {
			return true, nil
		}

		return false, err
	})
}
