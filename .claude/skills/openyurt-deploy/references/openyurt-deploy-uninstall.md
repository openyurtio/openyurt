# OpenYurt Uninstall & Rollback

If the user requests to roll back the OpenYurt deployment and revert edge nodes to standard worker nodes, execute these steps:

## Phase 1: Revert Node Conversion
1. Remove the autonomy annotation: `kubectl annotate node <node-name> node.openyurt.io/autonomy-duration-`
2. Remove the nodepool labels to trigger revert: 
   `kubectl label node <node-name> apps.openyurt.io/desired-nodepool-`
   `kubectl label node <node-name> apps.openyurt.io/nodepool-`
3. Wait for the `YurtNodeConversionController` to run the revert Job.
4. Verify the `openyurt.io/is-edge-worker` label is removed.

## Phase 2: Uninstall Components
1. Delete the NodePool CR: `kubectl delete nodepool <NODEPOOL_NAME>`
2. Uninstall yurt-manager: `helm uninstall yurt-manager -n kube-system`
3. Remove CRDs (if requested by user): `kubectl delete crd nodepools.apps.openyurt.io raven.openyurt.io`
