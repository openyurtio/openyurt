# Raven Troubleshooting

Decision tree for diagnosing cross-NodePool connectivity failures via Raven.

---

## 1. Raven Agent Not Running

```bash
kubectl -n kube-system get daemonset raven-agent
```

- **DaemonSet missing:** Install via Helm (see the flow document).
- **Pods not Ready:** Check logs:
  ```bash
  kubectl -n kube-system logs -l app=raven-agent --tail=30
  ```
  - **`libreswan` errors:** The L3 tunnel backend requires `libreswan` on
    each node. Deploy the same ephemeral check-ipsec Job from the
    Raven Flow reference to verify if it is missing.
    If missing, instruct the operator to install it via their remote
    management system:
    - Debian/Ubuntu: `sudo apt-get install -y libreswan`
    - CentOS/RHEL: `sudo yum install -y libreswan`

---

## 2. Gateway CR Not Electing Endpoints

```bash
kubectl get gateway <gw-name> -o yaml
```

Check `status.activeEndpoints`. If empty:

- **Are nodes labeled correctly?**
  ```bash
  kubectl get nodes -l raven.openyurt.io/gateway=<gw-name>
  ```
  If no nodes are returned, label them:
  ```bash
  kubectl label node -l apps.openyurt.io/nodepool=<pool-name> raven.openyurt.io/gateway=<gw-name>
  ```

- **Is the nodeSelector correct?** The `SetDefaultsGateway` webhook
  unconditionally sets `spec.nodeSelector` to
  `raven.openyurt.io/gateway: <gw-name>`. If you used
  `nodeSelectorTerms` with `apps.openyurt.io/nodepool`, the webhook
  silently overwrote it. Re-create the Gateway CR using the correct
  `matchLabels` shape.

---

## 3. PSK Mismatch

If tunnels are established but traffic is dropped:

```bash
kubectl -n kube-system get secret raven-ipsec-psk -o jsonpath='{.data.psk}' | base64 -d
```

Verify all Gateway CRs reference the same PSK secret. If the PSK was
rotated, delete and recreate the secret, then restart the raven-agent
DaemonSet:

```bash
kubectl -n kube-system rollout restart daemonset raven-agent
```

---

## 4. Cross-NodePool Ping Fails

If the autonomous connectivity test fails:

- **Check pod IPs are routable:** Verify the pod CIDR is configured for
  cross-node routing.
- **Check tunnel status:**
  ```bash
  kubectl -n kube-system logs -l app=raven-agent --tail=50 | grep -i "tunnel\|ipsec\|established"
  ```
- **Check firewall rules:** IPsec requires UDP ports 500 and 4500 to be
  open between nodes. Also check ESP (protocol 50).

---

## 5. Fine-Grained Override Not Taking Effect

If a Gateway-level annotation override (`raven.openyurt.io/enable-tunnel`
or `raven.openyurt.io/enable-proxy`) is not being respected:

```bash
kubectl get gateway <gw-name> -o jsonpath='{.metadata.annotations}'
```

- **Annotation present but ignored:** The `gatewaypickup` controller reads
  these annotations via `GetBoolAnnotation()` before falling back to
  `raven-global-config`. Ensure the raven-agent version supports this
  feature (introduced in the fine-grained network config PR).
- **Controller not reconciling:** Restart the raven-agent:
  ```bash
  kubectl -n kube-system rollout restart daemonset raven-agent
  ```
