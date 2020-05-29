package constants

const (
	// LabelEdgeWorker is used to identify if a node is a edge node ("true")
	// or a cloud node ("false")
	LabelEdgeWorker = "alibabacloud.com/is-edge-worker"

	// AnnotationAutonomy is used to identify if a node is automous
	AnnotationAutonomy = "node.beta.alibabacloud.com/autonomy"

	// YurtControllerManagerDeployment defines the yurt controller manager
	// deployment in yaml format
	YurtControllerManagerDeployment = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: yurt-controller-manager
  namespace: kube-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: yurt-controller-manager
  template:
    metadata:
      labels:
        app: yurt-controller-manager
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
      - name: yurt-controller-manager
        image: openyurt/yurt-controller-manager:latest
        command:
        - yurt-controller-manager	
`
	// ServantJobTemplate defines the servant job in yaml format
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
      containers:
      - name: yurtctl-servant
        image: openyurt/yurtctl-servant:edge
        command:
        - /bin/sh
        - -c
        args:
        - "sed -i 's|__kubernetes_service_host__|$(KUBERNETES_SERVICE_HOST)|g;s|__kubernetes_service_port_https__|$(KUBERNETES_SERVICE_PORT_HTTPS)|g;s|__node_name__|$(NODE_NAME)|g' /var/lib/openyurt/setup_edgenode && cp /var/lib/openyurt/setup_edgenode /tmp && nsenter -t 1 -m -u -n -i /var/tmp/setup_edgenode {{.action}} {{.provider}}"
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
