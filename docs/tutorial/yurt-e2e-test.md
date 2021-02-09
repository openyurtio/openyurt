# Yurt-e2e-test
This tutorial shows how to build and run the e2e test for OpenYurt. The test for node autonomy is still under development.

## Build
Let's build the e2e binary yurt-e2e-test as follow:
1) entering the openyurt work path:
```bash
 $cd openyurt
```

2) building the binary:
```bash
$ make e2e
```

## RUN Test
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

## Check test result
You can check test result in stdout or in file yurt-e2e-test-report_01.xml
