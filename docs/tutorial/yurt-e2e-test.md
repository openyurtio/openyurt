# Yurt-e2e-test
This tutorial shows how to build and run the e2e test for OpenYurt. The test for node autonomy is still under development.

If you have already set up the OpenYurt cluster, you can immediately follow the steps to test your cluster. Otherwise, you can quickly set up the OpenYurt at your local host with instructions at [Local Up OpenYurt](#local-up-openyurt)

## Run e2e tests
You can use the following command to run e2e tests. Assuming that you've entered the openyurt work path, run
```bash
make e2e-tests
```
This command will help you run e2e tests on OpenYurt cluster. It will use kubeconfig at `${KUBECONFIG}`, default path is `$HOME/.kube/config`. If there's no config found, it will end in error. The test result will show on the stdout.
```bash
Ran 5 of 5 Specs in 82.279 seconds
SUCCESS! -- 5 Passed | 0 Failed | 0 Pending | 0 Skipped
PASS
```
If you need further configuration for yurt-e2e-test to specify its behavior, you can [manually build and run e2e test](#Manually-Run-e2e-tests).

## Manually Run e2e tests
### Build
Let's build the e2e binary yurt-e2e-test as follows:
1) entering the openyurt work path:
```bash
 $cd openyurt
```

2) building the binary:
```bash
$ make e2e
```

### RUN Test
You can use yurt-e2e-test binary to test Openyurt.
1) If you run yurt-e2e-test without node autonomy test, such as:
```bash
$ ./_output/bin/darwin/amd64/yurt-e2e-test --kubeconfig=/root/.kube/config  --report-dir=./
```
This will run some basic tests after k8s is converted to openyurt. It refers to the operation of namespace and pod.

2) If you run yurt-e2e-test, and want to test yurt node autonomy on local machine. For example, it can run on minikube as follows. In this way, it depends on yourself to restart node. And yurt-e2e-test will wait for restarting node and checking pod status to test yurt node autonomy.
```
$ ./_output/bin/darwin/amd64/yurt-e2e-test --kubeconfig=/root/.kube/config  --report-dir=./  --enable-yurt-autonomy=true
```

3) If you want to test yurt node autonomy on aliyun ecs or aliyun ens with binary of yurt-e2e-test, TBD.

### Check test result
You can check test result in stdout or in file yurt-e2e-test-report_01.xml

## Local Up OpenYurt
If you don't have the openyurt cluster, you can run the bash shell [`local_up_openyurt.sh`](../../hack/local_up_openyurt.sh) to quickly set up the openyurt cluster at your local host.
Assuming that you've entered the openyurt work path, the commad is as simple as follows:
```bash
bash hack/local_up_openyurt.sh
```
Then you can use `kubectl` to interact with your OpenYurt cluster.

### What does the shell do for you
In summary, the `local_up_openyurt.sh` will use the local files under the openyurt work path to set up the cluster.  And you can specify the behavior of the shell through setting environment variables.

First, it will set up a docker container to build required binaries and images. The Arch and OS platform of built binaries and images is the same as your local host. For instance, if your local host is linux/amd64, then only binaries for linux/amd64 will be built. Built binaries and images will be saved under `_output` directory.

Second, it will use `kind` to set up the kubernetes cluster. You can set `KUBERNETESVERSION` to specify the kubernetes version to use. For instance, `export KUBERNETESVERSION=1.18` before running the shell will enable you to use kubernetes v1.18. In addition, you can set `NODES_NUM` to specify the number of nodes the cluster will contain.
>Note:
>1. The format of `KUBERNETESVERSION` is `1.xx`, other formats will not be accepted. The default KUBERNETESVERSION is `1.20`.
>2. `NODES_NUM` should not be less than 2. Finally, the cluster will contains one control-plane node and `NODES_NUM-1` woker nodes. The default `NODES_NUM` is 2.

Third, `yurtctl` will be used to convert the Kubernetes cluster into OpenYurt cluter. It will use images built earlier to deploy OpenYurt components, including `yurthub`, `yurt-controller-manager`, `yurt-tunnel-server` and `yurt-tunnel-agent`. You can disable `yurt-tunnel` through `export YURTTUNNEL=disable`.

By now, you've got the OpenYurt cluster at your local host and you can interact with it using `kubectl`. `kind` will automatically stored the kubeconfig at your `KUBECONFIG` path (default path is ${HOME}/.kube/config). If you already have the `KUBECONFIG` to interact with other clusters, `kind` will add a new context of openyurt cluster into the `KUBECONFIG` and automatically switch to it. You can manually switch back to the previous context using command `kubectl config use-context ${PREVIOUS_CONTEXT_NAME}`. For more details, you can see the [documentation](https://kubernetes.io/docs/tasks/access-application-cluster/configure-access-multiple-clusters/). You can store the kubeconfig at another path through setting `KIND_KUBECONFIG`.

### Reference
Reference gives descriptions of all used environment variables.

**REGION**
REGION affects the GOPROXY to use. You can set it to "cn" to use GOPROXY="https://goproxy.cn". Default value is "us", which means using GOPROXY="https://goproxy.io".

**KIND_KUBECONFIG**
KIND_KUBECONFIG represents the path to store the kubeconfig file of the cluster which is created by this shell. The default value is "$HOME/.kube/config".

**NODES_NUM**
NODES_NUM represents the number of nodes to set up in the new-created cluster. There are one control-plane node and NODES_NUM-1 worker nodes. Thus, NODES_NUM must not be less than 2. The default value is 2.

**KUBERNETESVERSION**
KUBERNETESVERSION declares the kubernetes version the cluster will use. The format is "1.XX". Now only 1.18, 1.19 and 1.20 are supported. The default value is 1.20.

**TIMEOUT**
TIMEOUT represents the time to wait for the kind control-plane, yurt-tunnel-server and yurt-tunnel-agent to be ready. If they are not ready after the duration, the shell will exit. The default value is 120s.

**YURTTUNNEL**
If set YURTTUNNEL=disable, the yurt-tunnel-agent and yurt-tunnel-server will not be deployed in the openyurt cluster. The default value is "enable".
