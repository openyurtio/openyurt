package constants

const (
	LabelEdgeWorker    = "alibabacloud.com/is-edge-worker"
	AnnotationAutonomy = "node.beta.alibabacloud.com/autonomy"

	YurtControllerManagerDeployment = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: yurt-ctrl-mgr
  namespace: kube-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: yurt-ctrl-mgr
  template:
    metadata:
      labels:
        app: yurt-ctrl-mgr
    spec:
      affinity:
        nodeAffinity:
          # we prefer allocating ecm on cloud node
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 1
            preference:
              matchExpressions:
              - key: alibabacloud.com/is-edge-worker
                operator: In
                values:
                - "false"
      containers:
      - name: yurt-ctrl-mgr
        image: openyurt/yurt-ctrl-mgr:latest
        command:
        - edge-controller-manager	
`
	ServantJobTemplate = `
apiVersion: batch/v1
kind: Job
metadata:
  name: {{.jobName}}
  namespace: kube-system
spec:
  template:
    spec:
      hostPID: true
      restartPolicy: OnFailure
      nodeName: {{.nodeName}}
      volumes:
      - name: host-var-tmp
        hostPath:
          path: /var/tmp
          type: Directory
      nodeSelector:
        alibabacloud.com/is-edge-worker: "true"
      containers:
      - name: yurtctl-servant
        image: openyurt/yurtctl-servant:latest
        command:
        - /bin/sh
        - -c
        args:
        - "sed -i 's|__kubernetes_service_host__|$(KUBERNETES_SERVICE_HOST)|g;s|__kubernetes_service_port_https__|$(KUBERNETES_SERVICE_PORT_HTTPS)|g;s|__node_name__|$(NODE_NAME)|g' /var/lib/openyurt/setup_edgenode && cp /var/lib/openyurt/setup_edgenode /tmp && nsenter -t 1 -m -u -n -i /var/tmp/setup_edgenode {{.provider}}"
        securityContext:
          privileged: true
        volumeMounts:
        - mountPath: /tmp
          name: host-var-tmp
        env:
        - name: NODE_NAME
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName
`
)
