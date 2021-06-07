module github.com/metal-stack/duros-controller

go 1.16

require (
	cloud.google.com/go v0.83.0 // indirect
	github.com/go-logr/logr v0.4.0
	github.com/go-logr/zapr v0.4.0 // indirect
	github.com/gogo/googleapis v1.4.1 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/metal-stack/duros-go v0.2.0
	github.com/metal-stack/v v1.0.3
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.13.0
	github.com/prometheus/common v0.27.0 // indirect
	go.uber.org/multierr v1.7.0 // indirect
	golang.org/x/crypto v0.0.0-20210513164829-c07d793c2f9a // indirect
	golang.org/x/time v0.0.0-20210220033141-f8bda1e9f3ba // indirect
	google.golang.org/grpc v1.38.0
	k8s.io/api v0.20.7
	k8s.io/apiextensions-apiserver v0.20.7 // indirect
	k8s.io/apimachinery v0.20.7
	k8s.io/client-go v0.20.7
	k8s.io/klog/v2 v2.9.0 // indirect
	k8s.io/utils v0.0.0-20210527160623-6fdb442a123b // indirect
	sigs.k8s.io/controller-runtime v0.8.3
)

replace github.com/coreos/etcd => github.com/coreos/etcd v3.3.25+incompatible
