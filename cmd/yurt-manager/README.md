# yurt-manager

In order to improve the management of all repos in OpenYurt, and reduce the complexity of deploying OpenYurt. we will create a new component named `yurt-manager` for managing all controllers and webhooks.

`yurt-manager` has built a framework for controllers that supports:
- crd definition
- controller logic
- webhook logic
- auto-issuing of webhook certificates
- helm chart generation
- client-api generation

the `yurt-manager` framework lets you easily add new CRD and their controller, webhook logic, avoiding a lot of repetitive development work.

## Code architecture

`yurt-manager` contains five structures:

| structure         | dirs                                | description                                                                                                                                       |
|:------------------|:------------------------------------|:--------------------------------------------------------------------------------------------------------------------------------------------------|
| add_controller.sh | `hack/make-rules/add_controller.sh` | Add a new CRD definition, including controller and Webhooks, you can use `make newcontroller` command                                             |
| cmd               | `cmd/yurt-manager`                  | yurt-manager entry function. In most cases, you do not need to modify it. Do not modify the directory structure!!!                                |
| apis              | `pkg/apis`                          | api definition, including the group and version directories. Do not modify the directory structure!!!                                             |
| controller        | `pkg/controller`                    | Process the logic of the crd controller. Do not modify the directory structure !!!                                                                |
| webhook           | `pkg/webhook`                       | Handle the logic of crd Webhooks. Do not modify the directory structure!!!                                                                        |
| charts            | `charts/openyurt`                   | openyurt helm chart, You can run the `make manifests` command to automatically modify the template file. Do not modify the directory structure!!! |
| manifest          | `_output/manifest`                 | temporary directory for converting helm chart. Do not modify the directory structure!!!                                                           |

The directory structure is as follows:

```text

├── hack
│   └── make-rules
│       ├── add_controller.sh
├── cmd
│   └─ yurt-manager
│       ├── README.md
│       └── main.go
├── pkg
│   ├─ apis
│   │   ├── addtoscheme_apps_v1beta1.go
│   │   ├── apis.go
│   │   └── apps
│   │       └── v1beta1
│   ├── controller
│   │   ├── controller.go
│   │   └─ {instance}
│   │       └── {instance}_controller.go
│   └── webhook
│      ├── add_{instance}.go
│      ├── {instance}
│      │   ├── mutating
│      │   └── validating
│      └── server.go
├── _output
│   └── manifest
│       ├── auto_generate
│       │   ├── crd
│       │   ├── rbac
│       │   └── webhook
│       └── kustomize
│           ├── crd
│           ├── default
│           ├── rbac
│           └── webhook
├── charts
│   └── openyurt
│       ├── Chart.yaml
│       ├── templates
│       │   ├── _helpers.tpl
│       │   ├── yurt-manager-auto-generated.yaml
│       │   └── yurt-manager.yaml
│       └── values.yaml

```

## How do I add a new CRD
### Record local changes to the repository

Before adding new CRDS, please use the git commit command to record local changes to the repository.

```shell
# cd $GOPATH/src/github.com/openyurtio/openyurt
# git add .
# git commit -m "prepare to add new crd"
```

### Add new crd

Assume that the new crd belongs to group `net`, version is `v1beta1`, kind is `Spider`, shortname is `sd`, and scope is `Cluster`

You can execute the following command:
``` shell
GROUP=net
VERSION=v1beta1
KIND=Spider
# make newcontroller GROUP=${GROUP} VERSION=${VERSION} KIND=${KIND} SHORTNAME=sd SCOPE=Cluster
```

Assume that the new crd belongs to group `iot`, version is `v1alpha1`, kind is `Device`, shortname is `dc`, and scope is `Namespaced`

You can execute the following command:
``` shell
GROUP=iot
VERSION=v1alpha1
KIND=Device
# make newcontroller GROUP=iot VERSION=v1alpha1 KIND=${KIND} SHORTNAME=dc SCOPE=Namespaced
```

After the preceding command is executed, you can run the `git status` command to check which files have been added and modified

```shell
# git status
```

### Update helm chart

```shell
# make manifests
```

`make manifests` command will automatically update file `charts/openyurt/templates/yurt-manager-auto-generated.yaml` . If you have other special configurations for `yurt-manger`, please update the `charts/openyurt/templates/yurt-manager.yaml` and `charts/openyurt/values.yaml` files.

Do not modify file `charts/openyurt/templates/yurt-manager-auto-generated.yaml` directly, because file `charts/openyurt/templates/yurt-manager-auto-generated.yaml` is overwritten every time command `make manifests` is executed

exec `git commit` to record local change

```shell
# git commit -m "add a new crd ${GROUP}/${VERSION}/${KIND} and controller ,webhook "
```
### Build and push yurt-manger image

Suppose the name of our image repository `edgetest-registry.cn-zhangjiakou.cr.aliyuncs.com/openyurt`

```shell
# make docker-push-yurt-manager IMAGE_REPO=edgetest-registry.cn-zhangjiakou.cr.aliyuncs.com/openyurt
```

`make docker-push-yurt-manager` automatically constructs docker images. The full image is named $IMAGE_REPO/yurt-manager:{git commit} and automatically pushed to the image repository.

### helm install

```shell
# cd $GOPATH/src/github.com/openyurtio/openyurt
# helm uninstall openyurt
# helm install openyurt ./openyurt --set yurtManager.image.registry=edgetest-registry.cn-zhangjiakou.cr.aliyuncs.com --set yurtManager.image.tag=v1.2.0-52ef527
```

Note !!!

You need to specify the correct `yurtManager.image.registry` and `yurtManager.image.tag` using the `--set` flag  when you execute `helm install` command.






