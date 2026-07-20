# OpenYurt Raven Configuration Flow

Follow these steps sequentially to configure Raven.

## Phase 1: Prerequisites
1. Verify OpenYurt is deployed: `kubectl get deployment yurt-manager -n kube-system`.
2. Generate a VPN Pre-Shared Key (PSK): 
   Run `openssl rand -hex 64`. Store the output.
3. Create a Secret with the PSK:
   `kubectl create secret generic raven-ipsec-psk --from-literal=psk=<GENERATED_PSK> -n kube-system`

## Phase 2: Deploy Raven Agent
1. Install the `raven-agent` DaemonSet via Helm:
   `helm upgrade --install raven-agent openyurt/raven-agent -n kube-system`
2. Wait for daemonset to be ready.

## Phase 3: Configure Gateways
For each NodePool that requires cross-region communication:
1. Ask the user for the expose type (`PublicIP` or `LoadBalancer`).
2. Label the nodes in the NodePool to join the gateway:
   `kubectl label node -l apps.openyurt.io/nodepool=<NODEPOOL_NAME> raven.openyurt.io/gateway=<NODEPOOL_NAME>-gw`
3. Create the Gateway CR:
   ```yaml
   apiVersion: raven.openyurt.io/v1beta1
   kind: Gateway
   metadata:
     name: <NODEPOOL_NAME>-gw
   spec:
     nodeSelector:
       matchLabels:
         raven.openyurt.io/gateway: <NODEPOOL_NAME>-gw
     exposeType: <EXPOSE_TYPE>
     tunnelConfig:
       Replicas: 1
     proxyConfig:
       Replicas: 1
   ```
4. Apply via `kubectl apply -f`.

## Phase 4: Autonomous Connectivity Test
To verify the L3 tunnel, execute the following script. Do not ask the user to manually verify; run this and parse the output.

```bash
#!/bin/bash
# Autonomous Raven Connectivity Test Script
echo "Deploying raven-test DaemonSet..."
cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: raven-test
  namespace: default
spec:
  selector:
    matchLabels:
      app: raven-test
  template:
    metadata:
      labels:
        app: raven-test
    spec:
      tolerations:
      - operator: Exists
      containers:
      - name: test
        image: busybox
        command: ["sleep", "3600"]
EOF

echo "Waiting for pods to be running..."
kubectl wait --for=condition=Ready pod -l app=raven-test --timeout=60s

PODS=$(kubectl get pods -l app=raven-test -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.status.podIP}{"\t"}{.spec.nodeName}{"\n"}{end}')
echo "$PODS" > /tmp/raven_pods.txt

# Extract two pods on different nodes
POD1_NAME=$(head -n 1 /tmp/raven_pods.txt | awk '{print $1}')
POD2_IP=$(tail -n 1 /tmp/raven_pods.txt | awk '{print $2}')

if [ -z "$POD1_NAME" ] || [ -z "$POD2_IP" ]; then
    echo "Failed to find two test pods. Ensure you have at least 2 nodes ready."
else
    echo "Pinging $POD2_IP from $POD1_NAME..."
    kubectl exec $POD1_NAME -- ping -c 3 $POD2_IP
    if [ $? -eq 0 ]; then
        echo "Raven L3 tunnel connectivity verified successfully!"
    else
        echo "Raven L3 tunnel connectivity failed."
    fi
fi

kubectl delete daemonset raven-test
```
