module github.ibm.com/istio-research/mc2019

go 1.12

require (
	github.com/go-logr/logr v0.1.0
	github.com/golang/protobuf v1.3.2
	github.com/onsi/ginkgo v1.7.0
	github.com/onsi/gomega v1.5.0
	istio.io/api v0.0.0-20190515205759-982e5c3888c6
	istio.io/pkg v0.0.0-20191014151857-998718349891
	k8s.io/api v0.0.0-20190409021203-6e4e0e4f393b
	k8s.io/apiextensions-apiserver v0.0.0-20190409022649-727a075fdec8
	k8s.io/apimachinery v0.0.0-20190404173353-6a84e37a896d
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	sigs.k8s.io/controller-runtime v0.2.2
)
