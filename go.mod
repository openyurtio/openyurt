module github.com/openyurtio/openyurt

go 1.16

require (
	github.com/Microsoft/go-winio v0.4.15
	github.com/aliyun/alibaba-cloud-sdk-go v1.61.579
	github.com/daviddengcn/go-colortext v1.0.0
	github.com/emicklei/go-restful v2.12.0+incompatible // indirect
	github.com/evanphx/json-patch v4.11.0+incompatible // indirect
	github.com/fsnotify/fsnotify v1.4.10-0.20200417215612-7f4cf4dd2b52 // indirect
	github.com/golangplus/testing v1.0.0 // indirect
	github.com/google/uuid v1.1.2
	github.com/gorilla/mux v1.7.4
	github.com/lithammer/dedent v1.1.0
	github.com/onsi/ginkgo v1.14.1
	github.com/onsi/gomega v1.10.2
	github.com/opencontainers/selinux v1.10.0
	github.com/openyurtio/yurt-app-manager-api v0.18.8
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.11.0
	github.com/spf13/cobra v1.1.3
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	github.com/vishvananda/netlink v1.1.1-0.20200603190939-5a869a71f0cb
	golang.org/x/sys v0.0.0-20210616094352-59db8d763f22
	google.golang.org/grpc v1.27.1
	gopkg.in/cheggaaa/pb.v1 v1.0.25
	k8s.io/api v0.22.3
	k8s.io/apimachinery v0.22.3
	k8s.io/apiserver v0.20.11
	k8s.io/client-go v0.22.3
	k8s.io/cluster-bootstrap v0.20.11
	k8s.io/component-base v0.20.11
	k8s.io/component-helpers v0.22.3
	k8s.io/controller-manager v0.20.11
	k8s.io/klog/v2 v2.30.0
	k8s.io/kube-controller-manager v0.20.11
	k8s.io/kubelet v0.20.11
	k8s.io/system-validators v1.2.0
	k8s.io/utils v0.0.0-20210930125809-cb0fa318a74b
	sigs.k8s.io/apiserver-network-proxy v0.0.15

)

replace (
	github.com/daviddengcn/go-colortext => github.com/daviddengcn/go-colortext v0.0.0-20160507010035-511bcaf42ccd
	github.com/googleapis/gnostic => github.com/googleapis/gnostic v0.4.1
	gopkg.in/cheggaaa/pb.v1 => gopkg.in/cheggaaa/pb.v1 v1.0.25
	k8s.io/api => k8s.io/api v0.20.11
	k8s.io/apimachinery => k8s.io/apimachinery v0.20.12-rc.0
	k8s.io/apiserver => k8s.io/apiserver v0.20.11
	k8s.io/client-go => k8s.io/client-go v0.20.11
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.20.11
	k8s.io/component-base => k8s.io/component-base v0.20.11
	k8s.io/component-helpers => k8s.io/component-helpers v0.22.3
	k8s.io/klog/v2 => k8s.io/klog/v2 v2.9.0
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.20.11
	sigs.k8s.io/apiserver-network-proxy => github.com/openyurtio/apiserver-network-proxy v1.18.8
	sigs.k8s.io/apiserver-network-proxy/konnectivity-client => sigs.k8s.io/apiserver-network-proxy/konnectivity-client v0.0.22
)
