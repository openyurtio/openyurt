package util

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/profile"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
)

const (
	// constants related dnat rules configmap
	YurttunnelServerDnatConfigMapNs = "kube-system"
	yurttunnelServerDnatDataKey     = "dnat-ports-pair"
)

var (
	YurttunnelServerDnatConfigMapName = fmt.Sprintf("%s-tunnel-server-cfg",
		strings.TrimRightFunc(projectinfo.GetProjectPrefix(), func(c rune) bool { return c == '-' }))
)

// RunMetaServer start a http server for serving metrics and pprof requests.
func RunMetaServer(addr string) {
	muxHandler := mux.NewRouter()
	muxHandler.Handle("/metrics", promhttp.Handler())

	// register handler for pprof
	profile.Install(muxHandler)

	metaServer := &http.Server{
		Addr:           addr,
		Handler:        muxHandler,
		MaxHeaderBytes: 1 << 20,
	}

	klog.InfoS("start handling meta requests(metrics/pprof)", "server endpoint", addr)
	go func() {
		err := metaServer.ListenAndServe()
		if err != nil {
			klog.ErrorS(err, "meta server could not listen")
		}
		klog.InfoS("meta server stopped listening", "server endpoint", addr)
	}()
}

// GetConfiguredDnatPorts returns the DNAT ports configured for tunnel server.
// NOTE: We only allow user to add dnat rule that uses insecure port as the destination port currently.
func GetConfiguredDnatPorts(client clientset.Interface, insecurePort string) ([]string, error) {
	ports := make([]string, 0)
	c, err := client.CoreV1().
		ConfigMaps(YurttunnelServerDnatConfigMapNs).
		Get(YurttunnelServerDnatConfigMapName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("configmap %s/%s is not found",
				YurttunnelServerDnatConfigMapNs,
				YurttunnelServerDnatConfigMapName)
		} else {
			return nil, fmt.Errorf("fail to get configmap %s/%s: %v",
				YurttunnelServerDnatConfigMapNs,
				YurttunnelServerDnatConfigMapName, err)
		}
	}

	pairStr, ok := c.Data[yurttunnelServerDnatDataKey]
	if !ok || len(pairStr) == 0 {
		return ports, nil
	}

	portsPair := strings.Split(pairStr, ",")
	for _, pair := range portsPair {
		portPair := strings.Split(pair, "=")
		if len(portPair) == 2 &&
			portPair[1] == insecurePort &&
			len(portPair[0]) != 0 {
			if portPair[0] != "10250" && portPair[0] != "10255" {
				ports = append(ports, portPair[0])
			}
		}
	}

	return ports, nil
}
