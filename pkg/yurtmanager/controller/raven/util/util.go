/*
Copyright 2023 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the License);
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an AS IS BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"strings"

	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// GetNodeInternalIP returns internal ip of the given `node`.
func GetNodeInternalIP(node corev1.Node) string {
	var ip string
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP && net.ParseIP(addr.Address) != nil {
			ip = addr.Address
			break
		}
	}
	return ip
}

// AddGatewayToWorkQueue adds the Gateway the reconciler's workqueue
func AddGatewayToWorkQueue(gwName string,
	q workqueue.RateLimitingInterface) {
	if gwName != "" {
		q.Add(reconcile.Request{
			NamespacedName: types.NamespacedName{Name: gwName},
		})
	}
}

func CheckServer(ctx context.Context, client client.Client) (enableProxy, enableTunnel bool) {
	var cm corev1.ConfigMap
	enableTunnel = false
	enableProxy = false
	err := client.Get(ctx, types.NamespacedName{Namespace: WorkingNamespace, Name: RavenGlobalConfig}, &cm)
	if err != nil {
		return enableProxy, enableTunnel
	}
	if val, ok := cm.Data[RavenEnableProxy]; ok && strings.ToLower(val) == "true" {
		enableProxy = true
	}
	if val, ok := cm.Data[RavenEnableTunnel]; ok && strings.ToLower(val) == "true" {
		enableTunnel = true
	}
	return enableProxy, enableTunnel
}

func AddDNSConfigmapToWorkQueue(q workqueue.RateLimitingInterface) {
	q.Add(reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: WorkingNamespace, Name: RavenProxyNodesConfig},
	})
}

func AddGatewayProxyInternalService(q workqueue.RateLimitingInterface) {
	q.Add(reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: WorkingNamespace, Name: GatewayProxyInternalService},
	})
}

func HashObject(o interface{}) string {
	data, _ := json.Marshal(o)
	var a interface{}
	err := json.Unmarshal(data, &a)
	if err != nil {
		klog.Errorf("unmarshal: %s", err.Error())
	}
	return computeHash(PrettyYaml(a))
}

func PrettyYaml(obj interface{}) string {
	bs, err := yaml.Marshal(obj)
	if err != nil {
		klog.Errorf("could not parse yaml, %v", err.Error())
	}
	return string(bs)
}

func computeHash(target string) string {
	hash := sha256.Sum224([]byte(target))
	return strings.ToLower(hex.EncodeToString(hash[:]))
}

func FormatName(name string) string {
	return strings.Join([]string{name, fmt.Sprintf("%08x", rand.Uint32())}, "-")
}
