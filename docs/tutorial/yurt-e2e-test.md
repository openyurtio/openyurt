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

2) If you run yurt-e2e-test, and want to test node autonomy, it is still under development.

## Check test result
You can check test result in stdout or in file yurt-e2e-test-report_01.xml
