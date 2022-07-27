# OpenYurt ClusterImage

Currently, `yurtadm init` is implemented by sealer v0.8.5 to create kubernetes master nodes.

## Install sealer

```bash
wget https://github.com/sealerio/sealer/releases/download/v0.8.5/sealer-v0.8.5-linux-amd64.tar.gz
tar -zxvf sealer-v0.8.5-linux-amd64.tar.gz -C /usr/bin
```

## Build your own OpenYurt Cluster

Modify the Kubefile to build your own OpenYurt cluster image.

### 1. Build OpenYurt Cluster Image

```bash
cd ./cluster-image

# build openyurt ClusterImage
sealer build -t registry-1.docker.io/your_dockerhub_username/openyurt-cluster:latest-k8s-1.21.14 -f Kubefile .

# push to dockerhub
sealer push registry-1.docker.io/your_dockerhub_username/openyurt-cluster:latest-k8s-1.21.14
```

### 2. Make a Clusterfile

A sample Clusterfile in ./Clusterfile

### 3. Run OpenYurt Cluster

```bash
sealer apply -f Clusterfile
```

Note: `yurtadm init` only creates master nodes. For worker nodes, you should use `yurtadm join`.