module github.com/alibaba/openyurt

go 1.13

require (
	github.com/Azure/go-ansiterm v0.0.0-20170929234023-d6e3b3328b78 // indirect
	github.com/NYTimes/gziphandler v1.1.1 // indirect
	github.com/Sirupsen/logrus v0.0.0-00010101000000-000000000000 // indirect
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/docker/docker v1.13.1 // indirect
	github.com/emicklei/go-restful v2.12.0+incompatible // indirect
	github.com/evanphx/json-patch v0.0.0-20200326221011-78cf02996493 // indirect
	github.com/go-openapi/spec v0.19.8 // indirect
	github.com/google/gofuzz v1.1.0 // indirect
	github.com/googleapis/gnostic v0.4.1 // indirect
	github.com/gorilla/mux v1.7.4
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/imdario/mergo v0.3.9 // indirect
	github.com/json-iterator/go v1.1.9 // indirect
	github.com/modern-go/reflect2 v1.0.1 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/pborman/uuid v1.2.0 // indirect
	github.com/prometheus/client_golang v1.0.0
	github.com/spf13/cobra v1.0.0
	github.com/spf13/pflag v1.0.5
	golang.org/x/time v0.0.0-20200416051211-89c76fbcd5d1 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.0.0 // indirect
	gopkg.in/square/go-jose.v2 v2.5.1 // indirect
	k8s.io/api v0.0.0-20191004102349-159aefb8556b
	k8s.io/apiextensions-apiserver v0.0.0-00010101000000-000000000000 // indirect
	k8s.io/apimachinery v0.17.3
	k8s.io/apiserver v0.0.0-20191015220424-a5d070e3855f
	k8s.io/client-go v11.0.1-0.20191004102930-01520b8320fc+incompatible
	k8s.io/cloud-provider v0.0.0-20191015223304-f52880ae9401 // indirect
	k8s.io/component-base v0.17.3
	k8s.io/klog v1.0.0
	k8s.io/kube-controller-manager v1.14.8 // indirect
	k8s.io/kube-openapi v0.0.0-20190228160746-b3a7cee44a30 // indirect
	k8s.io/kubernetes v1.18.3
	k8s.io/utils v0.0.0-20191114184206-e782cd3c129f
	sigs.k8s.io/structured-merge-diff v0.0.0-20190302045857-e85c7b244fd2 // indirect
	sigs.k8s.io/yaml v1.2.0 // indirect
)

replace (
	github.com/Sirupsen/logrus => github.com/sirupsen/logrus v1.6.0
	github.com/googleapis/gnostic => github.com/googleapis/gnostic v0.4.0
	github.com/prometheus/client_golang => github.com/prometheus/client_golang v0.9.2
	k8s.io/api => k8s.io/api v0.0.0-20191004102349-159aefb8556b
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.0.0-20191015221719-7d47edc353ef
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20191004074956-c5d2f014d689
	k8s.io/apiserver => k8s.io/apiserver v0.0.0-20191015220424-a5d070e3855f
	k8s.io/client-go => k8s.io/client-go v11.0.1-0.20191004102930-01520b8320fc+incompatible
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.0.0-20191015222949-226be42ae1a5
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20190228160746-b3a7cee44a30
	k8s.io/kubernetes => github.com/kubernetes/kubernetes v1.14.8
	sigs.k8s.io/structured-merge-diff => sigs.k8s.io/structured-merge-diff v0.0.0-20190302045857-e85c7b244fd2
)
