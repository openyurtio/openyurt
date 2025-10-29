# how to use a K8s-on-K8s cluster

## reference proposal

1. reference `docs/proposals/20240808-enhance-operational-efficiency-of-K8s-cluster-in-IDC.md`

## deploy tenant-K8s's control-plane components in host-K8s and deploy necessary configs and kube-proxy in tenant-K8s

1. modify `config.env` to customize user configuration

2. run `bash setup.sh`, which will setup a K8s-on-K8s cluster automatically

## yurtadm join a IDC node to K8s-on-K8s cluster

1. `make build WHAT=cmd/yurtadm`

2. `yurtadm join <TENANT_APISERVER_SERVICE:6443> --node-type=local --yurthub-binary-url=https://github.com/openyurtio/openyurt/releases/download/v1.x.y/yurthub-v1.x.y-linux-amd64.tar.gz --host-control-plane-addr=<HOST_CONTROL_PLANE_ADDR> --token=<TOKEN> --discovery-token-unsafe-skip-ca-verification --cri-socket=/run/containerd/containerd.sock --v=5`

## optional: deploy your own cni, coredns and so on.