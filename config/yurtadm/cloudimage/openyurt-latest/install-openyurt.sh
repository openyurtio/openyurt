#!/bin/bash

# Copyright 2022 The OpenYurt Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

echo "[INFO] Start installing OpenYurt."

## label node
kubectl label node $HOSTNAME openyurt.io/is-edge-worker=false

## install openyurt components
kubectl apply -f manifests/yurt-controller-manager.yaml
kubectl apply -f manifests/yurt-tunnel-agent.yaml
kubectl apply -f manifests/yurt-tunnel-server.yaml
kubectl apply -f manifests/yurt-app-manager.yaml
kubectl apply -f manifests/yurthub-cfg.yaml

## configure coredns
kubectl apply -f manifests/coredns.yaml
kubectl annotate svc kube-dns -n kube-system openyurt.io/topologyKeys='openyurt.io/nodepool'
kubectl scale --replicas=0 deployment/coredns -n kube-system

## configure kube-proxy
kubectl patch cm -n kube-system kube-proxy --patch '{"data": {"config.conf": "apiVersion: kubeproxy.config.k8s.io/v1alpha1\nbindAddress: 0.0.0.0\nfeatureGates:\n  EndpointSliceProxying: true\nbindAddressHardFail: false\nclusterCIDR: 100.64.0.0/10\nconfigSyncPeriod: 0s\nenableProfiling: false\nipvs:\n  excludeCIDRs:\n  - 10.103.97.2/32\n  minSyncPeriod: 0s\n  strictARP: false\nkind: KubeProxyConfiguration\nmode: ipvs\nudpIdleTimeout: 0s\nwinkernel:\n  enableDSR: false\nkubeconfig.conf:"}}'  && kubectl delete pod --selector k8s-app=kube-proxy -n kube-system

echo "[INFO] OpenYurt is successfully installed."