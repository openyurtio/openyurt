# Build an OpenYurt CloudImage

`yurtadm init` is implemented by sealer, you can modify the kubefile to make your own openyurt cloudimage.

```bash
cd openyurt-latest

# build cloudimage
sealer build -t registry-1.docker.io/openyurt/openyurt-cluster:latest-k8s-1198 -f Kubefile .

# push to dockerhub
sealer push registry-1.docker.io/openyurt/openyurt-cluster:latest-k8s-1198
```