module github.com/metal-stack/duros-controller

go 1.16

require (
	cloud.google.com/go v0.80.0 // indirect
	github.com/go-logr/logr v0.4.0
	github.com/go-logr/zapr v0.4.0 // indirect
	github.com/gogo/googleapis v1.4.1 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/metal-stack/duros-go v0.1.3-0.20210304125358-3da14d0a3b99
	github.com/metal-stack/v v1.0.3
	github.com/onsi/ginkgo v1.15.2
	github.com/onsi/gomega v1.11.0
	github.com/prometheus/common v0.20.0 // indirect
	go.uber.org/multierr v1.6.0 // indirect
	golang.org/x/crypto v0.0.0-20210322153248-0c34fe9e7dc2 // indirect
	golang.org/x/time v0.0.0-20210220033141-f8bda1e9f3ba // indirect
	google.golang.org/grpc v1.36.0
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
	honnef.co/go/tools v0.1.3 // indirect
	k8s.io/api v0.20.5
	k8s.io/apiextensions-apiserver v0.20.5 // indirect
	k8s.io/apimachinery v0.20.5
	k8s.io/client-go v0.20.5
	k8s.io/klog/v2 v2.8.0 // indirect
	k8s.io/utils v0.0.0-20210305010621-2afb4311ab10 // indirect
	sigs.k8s.io/controller-runtime v0.8.3
)

replace github.com/coreos/etcd => github.com/coreos/etcd v3.3.25+incompatible
