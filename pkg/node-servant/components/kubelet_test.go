package components

import (
	"regexp"
	"testing"

	enutil "github.com/openyurtio/openyurt/pkg/yurtctl/util/edgenode"
)

func Test_ParseKubeAdminServiceConfig(t *testing.T) {
	ct := `[Service]
EnvironmentFile=/var/lib/kubelet/kubeadm-flags.env
Environment='KUBELETCONFIG_ARGS=--config=/var/lib/kubelet/config.yaml'
Environment='KUBECONFIG_ARGS=--kubeconfig /etc/kubernetes/kubelet.conf --kubeconfig=/etc/kubernetes/kubelet.conf'
Environment='BOOTSTRAP_ARGS=--bootstrap-kubeconfig=/etc/kubernetes/bootstrap-kubelet.conf'
ExecStart=
ExecStart=/usr/local/bin/kubelet $KUBELETCONFIG_ARGS $KUBECONFIG_ARGS $BOOTSTRAP_ARGS $KUBELET_KUBEADM_AR`
	reg := regexp.MustCompile(kubeletConfigRegularExpression)
	res := reg.FindAllString(ct, -1)
	t.Log(res)
	enutil.GetSingleContentFromFile("", "")
}
