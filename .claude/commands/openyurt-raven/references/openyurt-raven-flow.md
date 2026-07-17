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
2. Create the Gateway CR:
   ```yaml
   apiVersion: raven.openyurt.io/v1beta1
   kind: Gateway
   metadata:
     name: <NODEPOOL_NAME>-gw
   spec:
     nodeSelector:
       nodeSelectorTerms:
         - matchExpressions:
             - key: apps.openyurt.io/nodepool
               operator: In
               values:
                 - <NODEPOOL_NAME>
     endpoints:
       - type: <EXPOSE_TYPE>
     tunnel:
       enable: true
     proxy:
       enable: true
   ```
3. Apply via `kubectl apply -f`.

## Phase 4: Connectivity Test
1. Deploy two test pods in different NodePools.
2. `kubectl exec` into one pod and `ping` the other pod's IP to verify L3 tunnel connectivity.
