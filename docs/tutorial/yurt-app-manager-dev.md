
# Build and deploy yurt-app-manager

In this tutorial, we will show how the yurt-app-manager helps developers to build yurt-app-manager using the source code.

## create and push yurt-app-manager image

Go to OpenYurt root directory:
```bash
$ cd $GOPATH/src/github.com/openyurtio/openyurt
```

You shoud first set global linux environment variables:
  - **IMAGE_REPO**: which stand for your own image registry for yurt-app-manager
  - **IMAGE_TAG**: which stand for yurt-app-manager image tag

For Example:
```bash
export IMAGE_REPO=registry.cn-hangzhou.aliyuncs.com/edge-kubernetes
export IMAGE_TAG="v0.3.0-"$(git rev-parse --short HEAD)
```

```bash
make clean
make release WHAT=cmd/yurt-app-manager ARCH=amd64 REGION=cn REPO=${IMAGE_REPO} GIT_VERSION=${IMAGE_TAG}
```

If everything goes right, we will get `${IMAGE_REPO}/yurt-app-manager:${GIT_VERSION}` image:

```bash
docker images ${IMAGE_REPO}/yurt-app-manager:${GIT_VERSION}
```

push yurt-app-manager image to your own registry
```bash
docker push ${IMAGE_REPO}/yurt-app-manager:${GIT_VERSION}
```
## Create yurt-app-manager yaml files

```bash
make gen-yaml WHAT=cmd/yurt-app-manager REPO=${IMAGE_REPO} GIT_VERSION=${IMAGE_TAG}
```

If everything goes right, we will have a yurt-app-manager.yaml files:
```bash
$ ls _output/yamls

yurt-app-manager.yaml
```

## install yurt-app-manager operator

```bash
kubectl apply -f _output/yamls/yurt-app-manager.yaml
```
The content of the `yurt-app-manager.yaml` file mainly includes three points:
- **NodePool CRD**
- **UnitedDeployment CRD**
- **yurtapp-controller-manager Deployment** which installed in `kube-system` namespaces

``` bash
kubectl get pod -n kube-system |grep yurt-app-manager
```

## How to Use

The Examples of NodePool and UnitedDeployment are in `config/yurt-app-manager/samples/` directory

