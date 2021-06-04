package controllers

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/metal-stack/duros-go"
	durosv2 "github.com/metal-stack/duros-go/api/duros/v2"

	storagev1 "github.com/metal-stack/duros-controller/api/v1"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1beta1"
	rbac "k8s.io/api/rbac/v1"
	storage "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	namespace = "kube-system"
)

var (
	hostPathDirectoryOrCreate       = v1.HostPathDirectoryOrCreate
	hostPathDirectory               = v1.HostPathDirectory
	mountPropagationHostToContainer = v1.MountPropagationHostToContainer
	mountPropagationBidirectional   = v1.MountPropagationBidirectional

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
			AllowedCapabilities: []v1.Capability{"SYS_ADMIN"},
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

	// CSIDriver
	csiDriver = storage.CSIDriver{
		ObjectMeta: metav1.ObjectMeta{
			Name: "csi.lightbitslabs.com",
		},
		Spec: storage.CSIDriverSpec{
			AttachRequired: boolp(true),
			PodInfoOnMount: boolp(true),
		},
	}
	// ServiceAccounts
	ctrlServiceAccount = v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "lb-csi-ctrl-sa",
			Namespace: namespace,
		},
	}

	nodeServiceAccount = v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "lb-csi-node-sa",
			Namespace: namespace,
		},
	}
	serviceAccounts = []v1.ServiceAccount{ctrlServiceAccount, nodeServiceAccount}

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
				Resources: []string{"volumeattachments"},
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
	defaultResourceLimits = v1.ResourceRequirements{
		Requests: v1.ResourceList{
			"cpu":    cpu100m,
			"memory": memory100m,
		},
		Limits: v1.ResourceList{
			"cpu":    cpu200m,
			"memory": memory200m,
		},
	}

	// Containers
	csiPluginContainer = v1.Container{
		Name:            "lb-csi-plugin",
		Image:           lbCSIPluginImage,
		ImagePullPolicy: v1.PullIfNotPresent,
		Args:            []string{"-P"},
		Env: []v1.EnvVar{
			{Name: "CSI_ENDPOINT", Value: "unix:///var/lib/csi/sockets/pluginproxy/csi.sock"},
			{Name: "KUBE_NODE_NAME", ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "spec.nodeName"}}},
			{Name: "LB_CSI_NODE_ID", Value: "$(KUBE_NODE_NAME).ctrl"},
			{Name: "LB_CSI_LOG_LEVEL", Value: "debug"},
			{Name: "LB_CSI_LOG_ROLE", Value: "controller"},
			{Name: "LB_CSI_LOG_FMT", Value: "text"},
			{Name: "LB_CSI_LOG_TIME", Value: "true"},
		},
		VolumeMounts: []v1.VolumeMount{
			{Name: socketDirVolume.Name, MountPath: "/var/lib/csi/sockets/pluginproxy/"},
		},
		Resources: defaultResourceLimits,
	}
	csiProvisionerContainer = v1.Container{
		Name:            "csi-provisioner",
		Image:           csiProvisionerImage,
		ImagePullPolicy: v1.PullIfNotPresent,
		Args:            []string{"--csi-address=$(ADDRESS)", "--v=4"},
		Env: []v1.EnvVar{
			{Name: "ADDRESS", Value: "/var/lib/csi/sockets/pluginproxy/csi.sock"},
		},
		VolumeMounts: []v1.VolumeMount{
			{Name: socketDirVolume.Name, MountPath: "/var/lib/csi/sockets/pluginproxy/"},
		},
		Resources: defaultResourceLimits,
	}
	csiAttacherContainer = v1.Container{
		Name:            "csi-attacher",
		Image:           csiAttacherImage,
		ImagePullPolicy: v1.PullIfNotPresent,
		Args:            []string{"--csi-address=$(ADDRESS)", "--v=5"},
		Env: []v1.EnvVar{
			{Name: "ADDRESS", Value: "/var/lib/csi/sockets/pluginproxy/csi.sock"},
		},
		VolumeMounts: []v1.VolumeMount{
			{Name: socketDirVolume.Name, MountPath: "/var/lib/csi/sockets/pluginproxy/"},
		},
		Resources: defaultResourceLimits,
	}
	csiResizerContainer = v1.Container{
		Name:            "csi-resizer",
		Image:           csiResizerImage,
		ImagePullPolicy: v1.PullIfNotPresent,
		Args:            []string{"--csi-address=$(ADDRESS)", "--v=4"},
		Env: []v1.EnvVar{
			{Name: "ADDRESS", Value: "/var/lib/csi/sockets/pluginproxy/csi.sock"},
		},
		VolumeMounts: []v1.VolumeMount{
			{Name: socketDirVolume.Name, MountPath: "/var/lib/csi/sockets/pluginproxy/"},
		},
		Resources: defaultResourceLimits,
	}
	snapshotControllerContainer = v1.Container{
		Name:            "snapshot-controller",
		Image:           snapshotControllerImage,
		ImagePullPolicy: v1.PullIfNotPresent,
		Args:            []string{"--leader-election=false", "--v=5"},
		Resources:       defaultResourceLimits,
	}
	csiSnapshotterContainer = v1.Container{
		Name:            "csi-snapshotter",
		Image:           csiSnapshotterImage,
		ImagePullPolicy: v1.PullIfNotPresent,
		Args:            []string{"--csi-address=$(ADDRESS)", "--leader-election=false", "--v=5"},
		Env: []v1.EnvVar{
			{Name: "ADDRESS", Value: "/var/lib/csi/sockets/pluginproxy/csi.sock"},
		},
		VolumeMounts: []v1.VolumeMount{
			{Name: socketDirVolume.Name, MountPath: "/var/lib/csi/sockets/pluginproxy/"},
		},
		Resources: defaultResourceLimits,
	}
	discoveryClientContainer = v1.Container{
		Name:            "lb-nvme-discovery-client",
		Image:           lbDiscoveryClientImage,
		ImagePullPolicy: v1.PullIfNotPresent,
		VolumeMounts: []v1.VolumeMount{
			{Name: deviceDirVolume.Name, MountPath: "/dev"},
			{Name: discoveryClientDirVolume.Name, MountPath: "/etc/discovery-client/discovery.d"},
		},
		SecurityContext: &v1.SecurityContext{
			Privileged: boolp(true),
			Capabilities: &v1.Capabilities{
				Add: []v1.Capability{"SYS_ADMIN"},
			},
			AllowPrivilegeEscalation: boolp(true),
		},
		Resources: defaultResourceLimits,
	}

	nodeInitContainer = v1.Container{
		Name:            "init-nvme-tcp",
		Image:           busyboxImage,
		ImagePullPolicy: v1.PullIfNotPresent,
		SecurityContext: &v1.SecurityContext{Privileged: boolp(true)},
		VolumeMounts: []v1.VolumeMount{
			{Name: modulesDirVolume.Name, MountPath: "/lib/modules", MountPropagation: &mountPropagationHostToContainer},
		},
		Command: []string{
			"/bin/sh",
			"-c",
			`[ -e /sys/module/nvme_tcp ] && modinfo nvme_tcp || { modinfo nvme_tcp && modprobe nvme_tcp ; } || { echo \"FAILED to load nvme-tcp kernel driver\" && exit 1 ; }`,
		},
		Resources: defaultResourceLimits,
	}

	csiPluginNodeContainer = v1.Container{
		Name:            "lb-csi-plugin",
		Image:           lbCSIPluginImage,
		ImagePullPolicy: v1.PullIfNotPresent,
		SecurityContext: &v1.SecurityContext{
			Privileged:               boolp(true),
			AllowPrivilegeEscalation: boolp(true),
			Capabilities:             &v1.Capabilities{Add: []v1.Capability{"SYS_ADMIN"}},
		},
		Args: []string{"-P"},
		Env: []v1.EnvVar{
			{Name: "CSI_ENDPOINT", Value: "unix:///csi/csi.sock"},
			{Name: "KUBE_NODE_NAME", ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "spec.nodeName"}}},
			{Name: "LB_CSI_NODE_ID", Value: "$(KUBE_NODE_NAME).node"},
			{Name: "LB_CSI_LOG_LEVEL", Value: "debug"},
			{Name: "LB_CSI_LOG_ROLE", Value: "node"},
			{Name: "LB_CSI_LOG_FMT", Value: "text"},
			{Name: "LB_CSI_LOG_TIME", Value: "true"},
		},
		VolumeMounts: []v1.VolumeMount{
			{Name: pluginDirVolume.Name, MountPath: "/csi"},
			{Name: podsMountDirVolume.Name, MountPath: "/var/lib/kubelet", MountPropagation: &mountPropagationBidirectional},
			{Name: deviceDirVolume.Name, MountPath: "/dev"},
			{Name: discoveryClientDirVolume.Name, MountPath: "/etc/discovery-client/discovery.d"},
		},
		Resources: defaultResourceLimits,
	}

	csiNodeDriverRegistrarContainer = v1.Container{
		Name:            "csi-node-driver-registrar",
		Image:           csiNodeDriverRegistrarImage,
		ImagePullPolicy: v1.PullIfNotPresent,
		Args: []string{
			"--v=4",
			"--csi-address=$(ADDRESS)",
			"--kubelet-registration-path=$(DRIVER_REG_SOCK_PATH)",
		},
		Env: []v1.EnvVar{
			{Name: "ADDRESS", Value: "/csi/csi.sock"},
			{Name: "DRIVER_REG_SOCK_PATH", Value: "/var/lib/kubelet/plugins/csi.lightbitslabs.com/csi.sock"},
			{Name: "KUBE_NODE_NAME", ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "spec.nodeName"}}},
		},
		Lifecycle: &v1.Lifecycle{PreStop: &v1.Handler{Exec: &v1.ExecAction{
			Command: []string{
				"/bin/sh",
				"-c",
				"rm -rf /registration/csi.lightbitslabs.com /registration/csi.lightbitslabs.com-reg.sock",
			},
		}}},
		VolumeMounts: []v1.VolumeMount{
			{Name: pluginDirVolume.Name, MountPath: "/csi"},
			{Name: registrationDirVolume.Name, MountPath: "/registration/"},
		},
		Resources: defaultResourceLimits,
	}

	// Volumes
	socketDirVolume = v1.Volume{
		Name:         "socket-dir",
		VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}},
	}
	discoveryClientDirVolume = v1.Volume{
		Name:         "discovery-client-dir",
		VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}},
	}
	registrationDirVolume = v1.Volume{
		Name: "registration-dir",
		VolumeSource: v1.VolumeSource{
			HostPath: &v1.HostPathVolumeSource{
				Path: "/var/lib/kubelet/plugins_registry/",
				Type: &hostPathDirectoryOrCreate,
			},
		},
	}
	pluginDirVolume = v1.Volume{
		Name: "plugin-dir",
		VolumeSource: v1.VolumeSource{
			HostPath: &v1.HostPathVolumeSource{
				Path: "/var/lib/kubelet/plugins/csi.lightbitslabs.com",
				Type: &hostPathDirectoryOrCreate,
			},
		},
	}
	podsMountDirVolume = v1.Volume{
		Name: "pods-mount-dir",
		VolumeSource: v1.VolumeSource{
			HostPath: &v1.HostPathVolumeSource{
				Path: "/var/lib/kubelet",
				Type: &hostPathDirectory,
			},
		},
	}
	deviceDirVolume = v1.Volume{
		Name: "device-dir",
		VolumeSource: v1.VolumeSource{
			HostPath: &v1.HostPathVolumeSource{
				Path: "/dev",
			},
		},
	}
	modulesDirVolume = v1.Volume{
		Name: "modules-dir",
		VolumeSource: v1.VolumeSource{
			HostPath: &v1.HostPathVolumeSource{
				Path: "/lib/modules",
			},
		},
	}

	// Controller StatefulSet
	controllerRoleLabels     = map[string]string{"app": "lb-csi-plugin", "role": "controller"}
	csiControllerStatefulSet = apps.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "lb-csi-controller", Namespace: namespace},
		Spec: apps.StatefulSetSpec{
			Selector:    &metav1.LabelSelector{MatchLabels: controllerRoleLabels},
			ServiceName: "lb-csi-ctrl-svc",
			Replicas:    int32p(1),
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: controllerRoleLabels},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						csiPluginContainer,
						csiProvisionerContainer,
						csiAttacherContainer,
						csiResizerContainer,
						snapshotControllerContainer,
						csiSnapshotterContainer,
					},
					ServiceAccountName: ctrlServiceAccount.Name,
					PriorityClassName:  "system-cluster-critical",
					Volumes: []v1.Volume{
						socketDirVolume,
					},
				},
			},
		},
	}

	// Node DaemonSet
	nodeRoleLabels   = map[string]string{"app": "lb-csi-node", "role": "node"}
	csiNodeDaemonSet = apps.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "lb-csi-node", Namespace: namespace},
		Spec: apps.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: nodeRoleLabels},
			UpdateStrategy: apps.DaemonSetUpdateStrategy{
				Type:          apps.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &apps.RollingUpdateDaemonSet{MaxUnavailable: &intstr.IntOrString{IntVal: 1}},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: nodeRoleLabels},
				Spec: v1.PodSpec{
					InitContainers: []v1.Container{
						nodeInitContainer,
					},
					Containers: []v1.Container{
						csiPluginNodeContainer,
						csiNodeDriverRegistrarContainer,
						discoveryClientContainer,
					},
					ServiceAccountName: nodeServiceAccount.Name,
					PriorityClassName:  "system-node-critical",
					HostNetwork:        true,
					Volumes: []v1.Volume{
						registrationDirVolume,
						pluginDirVolume,
						podsMountDirVolume,
						deviceDirVolume,
						modulesDirVolume,
						discoveryClientDirVolume,
					},
				},
			},
		},
	}

	storageClassCredentialsRef = "lb-csi-creds"
	storageClassTemplate       = storage.StorageClass{
		Provisioner:          csiDriver.ObjectMeta.Name,
		AllowVolumeExpansion: boolp(true),
		Parameters: map[string]string{
			"mgmt-scheme": "grpcs",
			"compression": "disabled",
			"csi.storage.k8s.io/controller-publish-secret-name":      storageClassCredentialsRef,
			"csi.storage.k8s.io/controller-publish-secret-namespace": namespace,
			"csi.storage.k8s.io/node-publish-secret-name":            storageClassCredentialsRef,
			"csi.storage.k8s.io/node-publish-secret-namespace":       namespace,
			"csi.storage.k8s.io/node-stage-secret-name":              storageClassCredentialsRef,
			"csi.storage.k8s.io/node-stage-secret-namespace":         namespace,
			"csi.storage.k8s.io/provisioner-secret-name":             storageClassCredentialsRef,
			"csi.storage.k8s.io/provisioner-secret-namespace":        namespace,
			"csi.storage.k8s.io/controller-expand-secret-name":       storageClassCredentialsRef,
			"csi.storage.k8s.io/controller-expand-secret-namespace":  namespace,
		},
	}
)

func (r *DurosReconciler) deployStorageClassSecret(ctx context.Context, credential *durosv2.Credential, adminKey []byte) error {
	key, err := extract(adminKey)
	if err != nil {
		return err
	}
	log := r.Log.WithName("storage-class")
	log.Info("deploy storage-class-secret")

	tokenLifetime := 360 * 24 * time.Hour
	token, err := duros.NewJWTTokenForCredential(r.Namespace, "duros-controller", credential, []string{credential.ProjectName + ":admin"}, tokenLifetime, key)
	if err != nil {
		return fmt.Errorf("unable to create jwt token:%w", err)
	}

	storageClassSecret := v1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: storageClassCredentialsRef, Namespace: namespace},
		Type:       "kubernetes.io/lb-csi",
		Data: map[string][]byte{
			"jwt": []byte(token),
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Shoot, &storageClassSecret, func() error { return nil })
	log.Info("storageclasssecret", "name", storageClassCredentialsRef, "operation", op)

	return err
}

func (r *DurosReconciler) deployStorageClass(ctx context.Context, projectID string, scs []storagev1.StorageClass) error {
	log := r.Log.WithName("storage-class")
	log.Info("deploy storage-class")

	// rm := r.Shoot.RESTMapper()
	// gvk, err := rm.ResourceFor(schema.GroupVersionResource{
	// 	Group:    "storage.k8s.io",
	// 	Resource: "CSIDriver",
	// })

	var csid storage.CSIDriver
	err := r.Shoot.Get(ctx, types.NamespacedName{Name: csiDriver.Name}, &csid)
	if err != nil {
		if errors.IsNotFound(err) {
			err = r.Shoot.Create(ctx, &csiDriver, &client.CreateOptions{})
			if err != nil {
				log.Error(err, "unable to create csidriver")
				return err
			}
		} else {
			return err
		}
	}

	for i := range psps {
		psp := psps[i]
		op, err := controllerutil.CreateOrUpdate(ctx, r.Shoot, &psp, func() error { return nil })
		if err != nil {
			return err
		}
		log.Info("psp", "name", psp.Name, "operation", op)
	}

	for i := range serviceAccounts {
		sa := serviceAccounts[i]
		op, err := controllerutil.CreateOrUpdate(ctx, r.Shoot, &sa, func() error { return nil })
		if err != nil {
			return err
		}
		log.Info("serviceaccount", "name", sa.Name, "operation", op)
	}

	for i := range clusterRoles {
		cr := clusterRoles[i]
		op, err := controllerutil.CreateOrUpdate(ctx, r.Shoot, &cr, func() error { return nil })
		if err != nil {
			return err
		}
		log.Info("clusterrole", "name", cr.Name, "operation", op)
	}

	for i := range clusterRoleBindings {
		crb := clusterRoleBindings[i]
		op, err := controllerutil.CreateOrUpdate(ctx, r.Shoot, &crb, func() error { return nil })
		if err != nil {
			return err
		}
		log.Info("clusterrolebindinding", "name", crb.Name, "operation", op)
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Shoot, &csiControllerStatefulSet, func() error {
		return nil
	})

	if err != nil {
		return err
	}
	log.Info("statefulset", "name", csiControllerStatefulSet.Name, "operation", op)

	op, err = controllerutil.CreateOrUpdate(ctx, r.Shoot, &csiNodeDaemonSet, func() error {
		csiNodeDaemonSet.Annotations["updated"] = time.Now().String()
		return nil
	})
	if err != nil {
		return err
	}
	log.Info("daemonset", "name", csiNodeDaemonSet.Name, "operation", op)

	for _, sc := range scs {
		storageClassName := sc.Name
		storageClassTemplate.ObjectMeta = metav1.ObjectMeta{Name: storageClassName}
		storageClassTemplate.Parameters["mgmt-endpoint"] = r.Endpoints.String()
		storageClassTemplate.Parameters["project-name"] = projectID
		storageClassTemplate.Parameters["replica-count"] = strconv.Itoa(sc.ReplicaCount)
		if sc.Compression {
			storageClassTemplate.Parameters["compression"] = "enabled"
		}
		op, err = controllerutil.CreateOrUpdate(ctx, r.Shoot, &storageClassTemplate, func() error { return nil })
		if err != nil {
			return err
		}
		log.Info("storageclass", "name", sc.Name, "operation", op)
	}

	return nil
}

func boolp(b bool) *bool {
	return &b
}
func int32p(i int32) *int32 {
	return &i
}
