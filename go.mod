module acnodal.io/puregw

go 1.16

require (
	github.com/go-logr/logr v0.4.0
	github.com/go-resty/resty/v2 v2.7.0
	github.com/golang/mock v1.5.0
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.15.0
	github.com/stretchr/testify v1.7.0
	github.com/vishvananda/netlink v1.1.0 // indirect
	k8s.io/api v0.22.1
	k8s.io/apimachinery v0.22.1
	k8s.io/client-go v0.22.1
	k8s.io/utils v0.0.0-20210820185131-d34e5cb4466e
	sigs.k8s.io/controller-runtime v0.10.0
	sigs.k8s.io/gateway-api v0.4.1
)
