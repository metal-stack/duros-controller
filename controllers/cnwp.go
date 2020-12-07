package controllers

import (
	"context"

	duros "github.com/metal-stack/duros-go"
	firewall "github.com/metal-stack/firewall-controller/api/v1"
	core "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (r *DurosReconciler) deployClusterWideNetworkPolicy(ctx context.Context, endpoints duros.EPs) error {
	log := r.Log.WithName("cnwp")
	log.Info("deploy cnwp")
	// apiVersion: metal-stack.io/v1
	// kind: ClusterwideNetworkPolicy
	// metadata:
	//   name: allow-to-storage
	//   namespace: firewall
	// spec:
	//   egress:
	//   - ports:
	// 	- port: 443
	// 	  protocol: TCP
	// 	- port: 4420
	// 	  protocol: TCP
	// 	- port: 8009
	// 	  protocol: TCP
	// 	to:
	// 	- cidr: 10.128.0.0/14
	tcp := core.ProtocolTCP
	https := intstr.FromInt(443)
	nvme := intstr.FromInt(4420)
	discovery := intstr.FromInt(8009)
	toIPBlocks := []networking.IPBlock{}
	for _, ep := range endpoints {
		ipb := networking.IPBlock{
			CIDR: ep.Host,
		}
		toIPBlocks = append(toIPBlocks, ipb)
	}
	cnwp := firewall.ClusterwideNetworkPolicy{
		ObjectMeta: v1.ObjectMeta{
			Name:      "allow-to-storage",
			Namespace: "firewall",
		},
		Spec: firewall.PolicySpec{
			Egress: []firewall.EgressRule{
				{
					Ports: []networking.NetworkPolicyPort{
						{Protocol: &tcp, Port: &https},
						{Protocol: &tcp, Port: &nvme},
						{Protocol: &tcp, Port: &discovery},
					},
					To: toIPBlocks,
				},
			},
		},
	}

	return r.createOrUpdate(ctx, log,
		types.NamespacedName{Name: cnwp.Name, Namespace: cnwp.Namespace},
		&cnwp,
	)
}
