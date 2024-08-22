|           title           | authors                                | reviewers   | creation-date | last-updated | status |
|:-------------------------:|----------------------------------------|-------------|---------------|--------------|--------|
| Build-iot-system-configuration-isolation-on-nodepool | @WoShiZhangmingyu | @LavenderQAQ | 2024-08-19    |    |        |

# Build-iot-system-configuration-isolation-on-nodepool
## Table of Contents

- [Build-iot-system-configuration-isolation-on-nodepool](#build-iot-system-configuration-isolation-on-nodepool)
  - [Table of Contents](#table-of-contents)
  - [Summary](#summary)
  - [Motivation](#motivation)
    - [Goals](#goals)
  - [Proposal](#proposal)
    - [User Stories](#user-stories)
    - [Implementation Details](#implementation-details)
    - [Test Plan](#test-plan)
  - [Implementation History](#implementation-history)


## Summary

Openyurt gave users the ability to customize iot systems, but it's currently not isolated enough for nodepool.
This proposal aims to provide multiple PlatformAdmin deployments within the same namespace, and to allow users the ability to customize the configuration of a nodepool.

## Motivation

OpenYurt extends the concept of nodepools on top of k8s, so an ideal deployment is that users can configure each nodepool iot system independently. With [#1435](https://github.com/openyurtio/openyurt/issues/1435) we can manipulate yurtappset to configure each nodepool individually. And service topology allows us to separate the traffic from each nodepool. With these two capabilities we can take a step closer to idealizing the current deployment model.
Currently, users can customize iot systems in [#1595](https://github.com/openyurtio/openyurt/issues/1595), but it's currently not isolated enough for nodepool.users can only deploy one nodepool through Platformadmin.

- old platformadmin(A Platformadmin is responsible for reconciling a nodepool)
 
![platformadmin-old](../img/20240819-build-iot-system-configuration-isolation-on-nodepool/platformadmin-old.png)

Suppose now you need to expand a node pool with the same configuration, the current plan is to create a new Platformadmin with the same configuration.Obviously, Obviously, the operability and reusability of this solution is poor.
One potential enhancement involves modifying the mapping between Platformadmin and nodepools to a one-to-many relationship, that is, changing the poolName in PlatformadminSpec to pools to correspond to multiple node pools.

- new platformadmin(One Platformadmin is responsible for reconciling multiple nodepools)

![platformadmin-new](../img/20240819-build-iot-system-configuration-isolation-on-nodepool/platformadmin-new.png)

### Goals

- Provide multiple PlatformAdmin deployments within the same namespace

- Allow users the ability to customize the configuration of a nodepool

- Add unit test for platform_ admin_controller and modify corresponding e2e test


## Proposal

### User Stories

- As a user,I wanted to customize configurations based on the node pool dimension, thereby achieving the reuse of both custom configurations and Platformadmin.

### Implementation Details

#### Modify CRD
Platformadmin needs to provide deployment for multiple node pools, so the original **poolName** has been changed to the **pools** , as follows:
~~~ 
pools:
    items:
        type: string
    type: array
~~~
A simple example:
~~~
apiVersion: iot.openyurt.io/v1alpha2
kind: PlatformAdmin
metadata: 
    name: edgex-sample
spec:
    version: minnesota 
    pools: 
    - hangzhou
EOF
~~~
#### Modify PlatformAdminSpec
Also change PoolName to pools:
~~~
type PlatformAdminSpec struct {
	Version string `json:"version,omitempty"`

	ImageRegistry string `json:"imageRegistry,omitempty"`

	Pools []string `json:"pools,omitempty"`

	ServiceType corev1.ServiceType `json:"serviceType,omitempty"`
	// +optional
	AdditionalService []ServiceTemplateSpec `json:"additionalServices,omitempty"`

	// +optional
	AdditionalDeployment []DeploymentTemplateSpec `json:"additionalDeployments,omitempty"`
}
~~~
#### Modify Platformadmin

Enhance the Reconcile logic of the platformadminController to accommodate multiple node pools, thereby enabling more refined resource management and scheduling.
for example:
~~~
for _, nodePool := range platformAdmin.Spec.Pools {
    pool := appsv1alpha1.Pool{
        Name:     nodePool,
        Replicas: pointer.Int32(1),
    }
    pool.NodeSelectorTerm.MatchExpressions = append(pool.NodeSelectorTerm.MatchExpressions,
        corev1.NodeSelectorRequirement{
            Key:      projectinfo.GetNodePoolLabel(),
            Operator: corev1.NodeSelectorOpIn,
            Values:   []string{nodePool},
        })
    flag := false
    for _, up := range yas.Spec.Topology.Pools {
        if up.Name == pool.Name {
            flag = true
            break
        }
    }
    if !flag {
        yas.Spec.Topology.Pools = append(yas.Spec.Topology.Pools, pool)
    }
}
~~~
### Test Plan

#### Unit Test
Create platformadmin_comtroler_test.go as a unit test file to verify if the corresponding service and yurtappset have been generated correctly.

#### E2E Test

Perform E2E testing only after ensuring that the unit test passes. Add test cases specifically to test configurations, such as verifying concurrent processing of multiple PlatformAdmin instances. Test in the local k8s environment simulated by kind to ensure that all parts of the system can work together

## Implementation History

- [ ] 8/19/2024: Draft proposal created
