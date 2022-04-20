/*
Copyright 2020 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package constants

const (
	//todo
	YurtAppManagerNodePool = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  annotations:
    controller-gen.kubebuilder.io/version: v0.7.0
  creationTimestamp: null
  name: nodepools.apps.openyurt.io
spec:
  group: apps.openyurt.io
  names:
    categories:
    - all
    kind: NodePool
    listKind: NodePoolList
    plural: nodepools
    shortNames:
    - np
    singular: nodepool
  scope: Cluster
  versions:
  - additionalPrinterColumns:
    - description: The type of nodepool
      jsonPath: .spec.type
      name: Type
      type: string
    - description: The number of ready nodes in the pool
      jsonPath: .status.readyNodeNum
      name: ReadyNodes
      type: integer
    - jsonPath: .status.unreadyNodeNum
      name: NotReadyNodes
      type: integer
    - jsonPath: .metadata.creationTimestamp
      name: Age
      type: date
    name: v1alpha1
    schema:
      openAPIV3Schema:
        description: NodePool is the Schema for the nodepools API
        properties:
          apiVersion:
            description: 'APIVersion defines the versioned schema of this representation
              of an object. Servers should convert recognized schemas to the latest
              internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources'
            type: string
          kind:
            description: 'Kind is a string value representing the REST resource this
              object represents. Servers may infer this from the endpoint the client
              submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds'
            type: string
          metadata:
            type: object
          spec:
            description: NodePoolSpec defines the desired state of NodePool
            properties:
              annotations:
                additionalProperties:
                  type: string
                description: 'If specified, the Annotations will be added to all nodes.
                  NOTE: existing labels with samy keys on the nodes will be overwritten.'
                type: object
              labels:
                additionalProperties:
                  type: string
                description: 'If specified, the Labels will be added to all nodes.
                  NOTE: existing labels with samy keys on the nodes will be overwritten.'
                type: object
              selector:
                description: A label query over nodes to consider for adding to the
                  pool
                properties:
                  matchExpressions:
                    description: matchExpressions is a list of label selector requirements.
                      The requirements are ANDed.
                    items:
                      description: A label selector requirement is a selector that
                        contains values, a key, and an operator that relates the key
                        and values.
                      properties:
                        key:
                          description: key is the label key that the selector applies
                            to.
                          type: string
                        operator:
                          description: operator represents a key's relationship to
                            a set of values. Valid operators are In, NotIn, Exists
                            and DoesNotExist.
                          type: string
                        values:
                          description: values is an array of string values. If the
                            operator is In or NotIn, the values array must be non-empty.
                            If the operator is Exists or DoesNotExist, the values
                            array must be empty. This array is replaced during a strategic
                            merge patch.
                          items:
                            type: string
                          type: array
                      required:
                      - key
                      - operator
                      type: object
                    type: array
                  matchLabels:
                    additionalProperties:
                      type: string
                    description: matchLabels is a map of {key,value} pairs. A single
                      {key,value} in the matchLabels map is equivalent to an element
                      of matchExpressions, whose key field is "key", the operator
                      is "In", and the values array contains only "value". The requirements
                      are ANDed.
                    type: object
                type: object
              taints:
                description: If specified, the Taints will be added to all nodes.
                items:
                  description: The node this Taint is attached to has the "effect"
                    on any pod that does not tolerate the Taint.
                  properties:
                    effect:
                      description: Required. The effect of the taint on pods that
                        do not tolerate the taint. Valid effects are NoSchedule, PreferNoSchedule
                        and NoExecute.
                      type: string
                    key:
                      description: Required. The taint key to be applied to a node.
                      type: string
                    timeAdded:
                      description: TimeAdded represents the time at which the taint
                        was added. It is only written for NoExecute taints.
                      format: date-time
                      type: string
                    value:
                      description: The taint value corresponding to the taint key.
                      type: string
                  required:
                  - effect
                  - key
                  type: object
                type: array
              type:
                description: The type of the NodePool
                type: string
            type: object
          status:
            description: NodePoolStatus defines the observed state of NodePool
            properties:
              nodes:
                description: The list of nodes' names in the pool
                items:
                  type: string
                type: array
              readyNodeNum:
                description: Total number of ready nodes in the pool.
                format: int32
                type: integer
              unreadyNodeNum:
                description: Total number of unready nodes in the pool.
                format: int32
                type: integer
            type: object
        type: object
    served: true
    storage: true
    subresources:
      status: {}
status:
  acceptedNames:
    kind: ""
    plural: ""
  conditions: []
  storedVersions: []
`
	YurtAppManagerUnitedDeployment = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  annotations:
    controller-gen.kubebuilder.io/version: v0.7.0
  creationTimestamp: null
  name: uniteddeployments.apps.openyurt.io
spec:
  group: apps.openyurt.io
  names:
    kind: UnitedDeployment
    listKind: UnitedDeploymentList
    plural: uniteddeployments
    shortNames:
    - ud
    singular: uniteddeployment
  scope: Namespaced
  versions:
  - additionalPrinterColumns:
    - description: The number of pods ready.
      jsonPath: .status.readyReplicas
      name: READY
      type: integer
    - description: The WorkloadTemplate Type.
      jsonPath: .status.templateType
      name: WorkloadTemplate
      type: string
    - description: CreationTimestamp is a timestamp representing the server time when
        this object was created. It is not guaranteed to be set in happens-before
        order across separate operations. Clients may not set this value. It is represented
        in RFC3339 form and is in UTC.
      jsonPath: .metadata.creationTimestamp
      name: AGE
      type: date
    name: v1alpha1
    schema:
      openAPIV3Schema:
        description: UnitedDeployment is the Schema for the uniteddeployments API
        properties:
          apiVersion:
            description: 'APIVersion defines the versioned schema of this representation
              of an object. Servers should convert recognized schemas to the latest
              internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources'
            type: string
          kind:
            description: 'Kind is a string value representing the REST resource this
              object represents. Servers may infer this from the endpoint the client
              submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds'
            type: string
          metadata:
            type: object
          spec:
            description: UnitedDeploymentSpec defines the desired state of UnitedDeployment.
            properties:
              revisionHistoryLimit:
                description: Indicates the number of histories to be conserved. If
                  unspecified, defaults to 10.
                format: int32
                type: integer
              selector:
                description: Selector is a label query over pods that should match
                  the replica count. It must match the pod template's labels.
                properties:
                  matchExpressions:
                    description: matchExpressions is a list of label selector requirements.
                      The requirements are ANDed.
                    items:
                      description: A label selector requirement is a selector that
                        contains values, a key, and an operator that relates the key
                        and values.
                      properties:
                        key:
                          description: key is the label key that the selector applies
                            to.
                          type: string
                        operator:
                          description: operator represents a key's relationship to
                            a set of values. Valid operators are In, NotIn, Exists
                            and DoesNotExist.
                          type: string
                        values:
                          description: values is an array of string values. If the
                            operator is In or NotIn, the values array must be non-empty.
                            If the operator is Exists or DoesNotExist, the values
                            array must be empty. This array is replaced during a strategic
                            merge patch.
                          items:
                            type: string
                          type: array
                      required:
                      - key
                      - operator
                      type: object
                    type: array
                  matchLabels:
                    additionalProperties:
                      type: string
                    description: matchLabels is a map of {key,value} pairs. A single
                      {key,value} in the matchLabels map is equivalent to an element
                      of matchExpressions, whose key field is "key", the operator
                      is "In", and the values array contains only "value". The requirements
                      are ANDed.
                    type: object
                type: object
              topology:
                description: Topology describes the pods distribution detail between
                  each of pools.
                properties:
                  pools:
                    description: Contains the details of each pool. Each element in
                      this array represents one pool which will be provisioned and
                      managed by UnitedDeployment.
                    items:
                      description: Pool defines the detail of a pool.
                      properties:
                        name:
                          description: Indicates pool name as a DNS_LABEL, which will
                            be used to generate pool workload name prefix in the format
                            '<deployment-name>-<pool-name>-'. Name should be unique
                            between all of the pools under one UnitedDeployment. Name
                            is NodePool Name
                          type: string
                        nodeSelectorTerm:
                          description: Indicates the node selector to form the pool.
                            Depending on the node selector, pods provisioned could
                            be distributed across multiple groups of nodes. A pool's
                            nodeSelectorTerm is not allowed to be updated.
                          properties:
                            matchExpressions:
                              description: A list of node selector requirements by
                                node's labels.
                              items:
                                description: A node selector requirement is a selector
                                  that contains values, a key, and an operator that
                                  relates the key and values.
                                properties:
                                  key:
                                    description: The label key that the selector applies
                                      to.
                                    type: string
                                  operator:
                                    description: Represents a key's relationship to
                                      a set of values. Valid operators are In, NotIn,
                                      Exists, DoesNotExist. Gt, and Lt.
                                    type: string
                                  values:
                                    description: An array of string values. If the
                                      operator is In or NotIn, the values array must
                                      be non-empty. If the operator is Exists or DoesNotExist,
                                      the values array must be empty. If the operator
                                      is Gt or Lt, the values array must have a single
                                      element, which will be interpreted as an integer.
                                      This array is replaced during a strategic merge
                                      patch.
                                    items:
                                      type: string
                                    type: array
                                required:
                                - key
                                - operator
                                type: object
                              type: array
                            matchFields:
                              description: A list of node selector requirements by
                                node's fields.
                              items:
                                description: A node selector requirement is a selector
                                  that contains values, a key, and an operator that
                                  relates the key and values.
                                properties:
                                  key:
                                    description: The label key that the selector applies
                                      to.
                                    type: string
                                  operator:
                                    description: Represents a key's relationship to
                                      a set of values. Valid operators are In, NotIn,
                                      Exists, DoesNotExist. Gt, and Lt.
                                    type: string
                                  values:
                                    description: An array of string values. If the
                                      operator is In or NotIn, the values array must
                                      be non-empty. If the operator is Exists or DoesNotExist,
                                      the values array must be empty. If the operator
                                      is Gt or Lt, the values array must have a single
                                      element, which will be interpreted as an integer.
                                      This array is replaced during a strategic merge
                                      patch.
                                    items:
                                      type: string
                                    type: array
                                required:
                                - key
                                - operator
                                type: object
                              type: array
                          type: object
                        patch:
                          description: Indicates the patch for the templateSpec Now
                            support strategic merge path :https://kubernetes.io/docs/tasks/manage-kubernetes-objects/update-api-object-kubectl-patch/#notes-on-the-strategic-merge-patch
                            Patch takes precedence over Replicas fields If the Patch
                            also modifies the Replicas, use the Replicas value in
                            the Patch
                          type: object
                        replicas:
                          description: Indicates the number of the pod to be created
                            under this pool.
                          format: int32
                          type: integer
                        tolerations:
                          description: Indicates the tolerations the pods under this
                            pool have. A pool's tolerations is not allowed to be updated.
                          items:
                            description: The pod this Toleration is attached to tolerates
                              any taint that matches the triple <key,value,effect>
                              using the matching operator <operator>.
                            properties:
                              effect:
                                description: Effect indicates the taint effect to
                                  match. Empty means match all taint effects. When
                                  specified, allowed values are NoSchedule, PreferNoSchedule
                                  and NoExecute.
                                type: string
                              key:
                                description: Key is the taint key that the toleration
                                  applies to. Empty means match all taint keys. If
                                  the key is empty, operator must be Exists; this
                                  combination means to match all values and all keys.
                                type: string
                              operator:
                                description: Operator represents a key's relationship
                                  to the value. Valid operators are Exists and Equal.
                                  Defaults to Equal. Exists is equivalent to wildcard
                                  for value, so that a pod can tolerate all taints
                                  of a particular category.
                                type: string
                              tolerationSeconds:
                                description: TolerationSeconds represents the period
                                  of time the toleration (which must be of effect
                                  NoExecute, otherwise this field is ignored) tolerates
                                  the taint. By default, it is not set, which means
                                  tolerate the taint forever (do not evict). Zero
                                  and negative values will be treated as 0 (evict
                                  immediately) by the system.
                                format: int64
                                type: integer
                              value:
                                description: Value is the taint value the toleration
                                  matches to. If the operator is Exists, the value
                                  should be empty, otherwise just a regular string.
                                type: string
                            type: object
                          type: array
                      required:
                      - name
                      type: object
                    type: array
                type: object
              workloadTemplate:
                description: WorkloadTemplate describes the pool that will be created.
                properties:
                  deploymentTemplate:
                    description: Deployment template
                    properties:
                      metadata:
                        x-kubernetes-preserve-unknown-fields: true
                      spec:
                        x-kubernetes-preserve-unknown-fields: true
                    required:
                    - spec
                    type: object
                  statefulSetTemplate:
                    description: StatefulSet template
                    properties:
                      metadata:
                        x-kubernetes-preserve-unknown-fields: true
                      spec:
                        x-kubernetes-preserve-unknown-fields: true
                    required:
                    - spec
                    type: object
                type: object
            required:
            - selector
            type: object
          status:
            description: UnitedDeploymentStatus defines the observed state of UnitedDeployment.
            properties:
              collisionCount:
                description: Count of hash collisions for the UnitedDeployment. The
                  UnitedDeployment controller uses this field as a collision avoidance
                  mechanism when it needs to create the name for the newest ControllerRevision.
                format: int32
                type: integer
              conditions:
                description: Represents the latest available observations of a UnitedDeployment's
                  current state.
                items:
                  description: UnitedDeploymentCondition describes current state of
                    a UnitedDeployment.
                  properties:
                    lastTransitionTime:
                      description: Last time the condition transitioned from one status
                        to another.
                      format: date-time
                      type: string
                    message:
                      description: A human readable message indicating details about
                        the transition.
                      type: string
                    reason:
                      description: The reason for the condition's last transition.
                      type: string
                    status:
                      description: Status of the condition, one of True, False, Unknown.
                      type: string
                    type:
                      description: Type of in place set condition.
                      type: string
                  type: object
                type: array
              currentRevision:
                description: CurrentRevision, if not empty, indicates the current
                  version of the UnitedDeployment.
                type: string
              observedGeneration:
                description: ObservedGeneration is the most recent generation observed
                  for this UnitedDeployment. It corresponds to the UnitedDeployment's
                  generation, which is updated on mutation by the API Server.
                format: int64
                type: integer
              poolReplicas:
                additionalProperties:
                  format: int32
                  type: integer
                description: Records the topology detail information of the replicas
                  of each pool.
                type: object
              readyReplicas:
                description: The number of ready replicas.
                format: int32
                type: integer
              replicas:
                description: Replicas is the most recently observed number of replicas.
                format: int32
                type: integer
              templateType:
                description: TemplateType indicates the type of PoolTemplate
                type: string
            required:
            - currentRevision
            - replicas
            - templateType
            type: object
        type: object
    served: true
    storage: true
    subresources:
      status: {}
status:
  acceptedNames:
    kind: ""
    plural: ""
  conditions: []
  storedVersions: []
`
	YurtAppManagerRole = `
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: yurt-app-leader-election-role
  namespace: kube-system
rules:
- apiGroups:
  - ""
  resources:
  - configmaps
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
- apiGroups:
  - ""
  resources:
  - configmaps/status
  verbs:
  - get
  - update
  - patch
- apiGroups:
  - ""
  resources:
  - events
  verbs:
  - create
    `
	YurtAppManagerClusterRole = `
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  creationTimestamp: null
  name: yurt-app-manager-role
rules:
- apiGroups:
  - admissionregistration.k8s.io
  resources:
  - mutatingwebhookconfigurations
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - admissionregistration.k8s.io
  resources:
  - validatingwebhookconfigurations
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - apps
  resources:
  - controllerrevisions
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - apps
  resources:
  - deployments
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - apps
  resources:
  - deployments/status
  verbs:
  - get
  - patch
  - update
- apiGroups:
  - apps
  resources:
  - statefulsets
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - apps
  resources:
  - statefulsets/status
  verbs:
  - get
  - patch
  - update
- apiGroups:
  - apps.openyurt.io
  resources:
  - nodepools
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - apps.openyurt.io
  resources:
  - nodepools/status
  verbs:
  - get
  - patch
  - update
- apiGroups:
  - apps.openyurt.io
  resources:
  - uniteddeployments
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - apps.openyurt.io
  resources:
  - uniteddeployments/status
  verbs:
  - get
  - patch
  - update
- apiGroups:
  - coordination.k8s.io
  resources:
  - leases
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - ""
  resources:
  - events
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - ""
  resources:
  - nodes
  verbs:
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - ""
  resources:
  - persistentvolumeclaims
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - ""
  resources:
  - pods
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - ""
  resources:
  - secrets
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
`
	YurtAppManagerRolebinding = `
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: yurt-app-leader-election-rolebinding
  namespace: kube-system
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: yurt-app-leader-election-role
subjects:
- kind: ServiceAccount
  name: default
  namespace: kube-system
`
	YurtAppManagerClusterRolebinding = `
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: yurt-app-manager-rolebinding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: yurt-app-manager-role
subjects:
- kind: ServiceAccount
  name: default
  namespace: kube-system
`
	//todo
	YurtAppManagerSecret = `
apiVersion: v1
kind: Secret
metadata:
  name: yurt-app-webhook-certs
  namespace: kube-system
`
	YurtAppManagerService = `
apiVersion: v1
kind: Service
metadata:
  name: yurt-app-webhook-service
  namespace: kube-system
spec:
  ports:
  - port: 443
    targetPort: 9876
  selector:
    control-plane: yurt-app-manager
`
	YurtAppManagerDeployment = `
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    control-plane: yurt-app-manager
  name: yurt-app-manager
  namespace: kube-system
spec:
  replicas: 2
  selector:
    matchLabels:
      control-plane: yurt-app-manager
  template:
    metadata:
      labels:
        control-plane: yurt-app-manager
    spec:
      containers:
      - args:
        - --enable-leader-election
        - --v=4
        command:
        - /usr/local/bin/yurt-app-manager
        image: {{.image}}
        imagePullPolicy: IfNotPresent
        name: manager
        ports:
        - containerPort: 9443
          name: webhook-server
          protocol: TCP
        volumeMounts:
        - mountPath: /tmp/k8s-webhook-server/serving-certs
          name: cert
          readOnly: true
      nodeSelector:
        openyurt.io/is-edge-worker: "false"
        beta.kubernetes.io/os: linux
      priorityClassName: system-node-critical
      terminationGracePeriodSeconds: 10
      tolerations:
      - effect: NoSchedule
        key: node-role.openyurt.io/addon
        operator: Exists
      volumes:
      - name: cert
        secret:
          defaultMode: 420
          secretName: yurt-app-webhook-certs
`
	//todo
	YurtAppManagerMutatingWebhookConfiguration = `
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: yurt-app-mutating-webhook-configuration
webhooks:
- clientConfig:
    caBundle: Cg==
    service:
      name: yurt-app-webhook-service
      namespace: kube-system
      path: /mutate-apps-openyurt-io-v1alpha1-nodepool
  failurePolicy: Fail
  admissionReviewVersions: ["v1", "v1beta1"]
  sideEffects: None
  name: mnodepool.kb.io
  rules:
  - apiGroups:
    - apps.openyurt.io
    apiVersions:
    - v1alpha1
    operations:
    - CREATE
    - UPDATE
    resources:
    - nodepools
- clientConfig:
    caBundle: Cg==
    service:
      name: yurt-app-webhook-service
      namespace: kube-system
      path: /mutate-apps-openyurt-io-v1alpha1-uniteddeployment
  failurePolicy: Fail
  admissionReviewVersions: ["v1", "v1beta1"]
  sideEffects: None
  name: muniteddeployment.kb.io
  rules:
  - apiGroups:
    - apps.openyurt.io
    apiVersions:
    - v1alpha1
    operations:
    - CREATE
    - UPDATE
    resources:
    - uniteddeployments
`
	//todo
	YurtAppManagerValidatingWebhookConfiguration = `
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: yurt-app-validating-webhook-configuration
webhooks:
- clientConfig:
    caBundle: Cg==
    service:
      name: yurt-app-webhook-service
      namespace: kube-system
      path: /validate-apps-openyurt-io-v1alpha1-nodepool
  failurePolicy: Fail
  admissionReviewVersions: ["v1", "v1beta1"]
  sideEffects: None
  name: vnodepool.kb.io
  rules:
  - apiGroups:
    - apps.openyurt.io
    apiVersions:
    - v1alpha1
    operations:
    - CREATE
    - UPDATE
    - DELETE
    resources:
    - nodepools
- clientConfig:
    caBundle: Cg==
    service:
      name: yurt-app-webhook-service
      namespace: kube-system
      path: /validate-apps-openyurt-io-v1alpha1-uniteddeployment
  failurePolicy: Fail
  admissionReviewVersions: ["v1", "v1beta1"]
  sideEffects: None
  name: vuniteddeployment.kb.io
  rules:
  - apiGroups:
    - apps.openyurt.io
    apiVersions:
    - v1alpha1
    operations:
    - CREATE
    - UPDATE
    resources:
    - uniteddeployments
`
)
