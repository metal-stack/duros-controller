module github.com/metal-stack/duros-controller

go 1.15

require (
	github.com/go-logr/logr v0.3.0
	github.com/gogo/googleapis v1.4.0 // indirect
	github.com/metal-stack/duros-go v0.0.0-20201206100937-c4a2f8bd9014
	github.com/metal-stack/firewall-controller v0.3.1
	github.com/metal-stack/v v1.0.2
	github.com/onsi/ginkgo v1.14.2
	github.com/onsi/gomega v1.10.3
	github.com/rakyll/statik v0.1.7
	golang.org/x/net v0.0.0-20201202161906-c7110b5ffcbb // indirect
	golang.org/x/sys v0.0.0-20201204225414-ed752295db88 // indirect
	google.golang.org/genproto v0.0.0-20201204160425-06b3db808446 // indirect
	google.golang.org/grpc v1.34.0
	k8s.io/api v0.19.4
	k8s.io/apiextensions-apiserver v0.19.4
	k8s.io/apimachinery v0.19.4
	k8s.io/client-go v0.19.4
	sigs.k8s.io/controller-runtime v0.6.4
	sigs.k8s.io/yaml v1.2.0
)

replace github.com/coreos/etcd => github.com/coreos/etcd v3.3.25+incompatible
