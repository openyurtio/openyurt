# Separate yurt-manager clients

|              title              | authors   | reviewers | creation-date | last-updated | status |
| :-----------------------------: | --------- | --------- | ------------- | ------------ | ------ |
| Separate yurt-manager clients | @luc99hen |   @rambohe-ch        | 2024-05-17    |              |        |

<!-- TOC -->
* [Separate yurt-manager clients](#Separate-yurt-manager-clients)
  * [Summary](#summary)
  * [Motivation](#motivation)
    * [Goals](#goals)
    * [Non-Goals/Future Work](#non-goals)
  * [Proposal](#proposal)
  * [Implementation History](#implementation-history)
<!-- TOC -->

## Summary

Yurt-manager is an important component in cloud environment for OpenYurt which contains multiple controllers and webhooks. Currently, those controllers and webhooks share one client and one set of RBAC (yurt-manager-role/yurt-manager-role-binding/yurt-manager-sa) which grows bigger as we add more function into yurt-manager. This mechanism makes a controller has access it shouldn't has. Furthermore, sharing one client and user agent makes it difficult to find out the request is from which controller from the audit logs.

This proposal aims to address this issue by separating clients for different controllers and webhooks.

## Motivation

1. Sharing RBAC among controller/webhooks violates the principle of least authority.
2. Sharing user agent is hard to trace the request from audit logs.

### Goals

1. Separate RBAC for different controllers and webhooks and make each has its own identity in Kubernetes.
2. Separate User agents for different controllers and webhooks.
3. Restrict each controller/webhook to only the permissions it may use.

### Non-Goals

1. Divide Yurt-manager into multiple components.

## Proposal

### Design principals

1. Compatible with currently used controller-runtime framework. Developers of yurt-manager will not notice the changes underhood.
2. Follow the principle of least authority. One controller/webhook should not be granted permissions it will not use.

### Critical Questions

1. What's the granularity of division?

The structure of Yurt-manager is relatively complex. Yurt-manager is made up of controllers and webhooks. Some controllers may also have several sub-controllers. Also, for RBAC division, yurt-manager itself needs some base permissions to manage the whole component.

```
yurt-manager (base)
- controllers
  - yurt-app-set
  - yurt-coordiator
    - yurt-coordinator-cert
	- pod-binding
  - ...
- webhooks
  - yurt-app-set
  - ...
```

In order to be compatible with the existing system, and also easy to manage permissions, we decide to divide the RBAC in this pattern

```
base: used in the entire life-cycle of yurt-manager, such as controller initialization and webhook server set up
yurt-app-set(controller/webhook): used for yurt-app-set controller and webhook
yurt-coordinator-cert(controller): used for yurt-coordinator-cert
pod-binding(controller): used for pod-binding
...
```

2. How to use different RBAC for different controller/webhooks?

After different RBACs are prepared, we should make sure they are properly used by different controllers. We consider two feasible approaches here to achieve this goal.

1) User impersonation

Kubernetes provides a [mechanism](https://kubernetes.io/docs/reference/access-authn-authz/authentication/#user-impersonation) that one user can act as another user through impersonation headers. These let requests manually override the user info a request authenticates as. 

2) Token override

We can override the client config' token with the prepared ServiceAccount token which has been bound to prescribed roles or cluster roles.

Considering the additional complexity of the first approach, we choose the second one to override the token directly when building the client for each controller/webhook.

### Solution Introduction

The whole solution consists of two steps: 

1. Generate RBAC

First, prepare RBAC artifacts including role/clusterrole, rolebindidng/clusterrolebinding and serviceaccount for controllers and webhooks in yurt-manager. The specific steps are as following:

1) Use bash scripts to search through the pkg/yurt-manager folder and find the components which need independent RBAC.
2) Generate clusterrole and roles for those components one by one with `controller-gen`.
3) Use bash scripts to complement the corresponding rolebinding/clusterrolebinding and serviceaccount artifacts.
4) Gather all generated materials into one yaml in charts/yurt-manager/templates/yurt-manager-auto-generated.yaml with `kustomize`.

Those steps above are performed automatically when you run `make manifests` command. Basically, yurt-manager developers don't need to know these implementations underhood.

2. Prepare client

Currently, all controllers use a client from `manager.GetClient()` which is provided by controller runtime framework directly. However the framework [doesn't provide any interface](https://github.com/kubernetes-sigs/controller-runtime/issues/2822) to provide different clients by controllers. Therefore, we have to implement a client wrapper to to construct a new client for every component based on the basic client.

```go 
func GetClientByControllerNameOrDie(mgr manager.Manager, controllerName, namespace string) client.Client {
	// if controllerName is empty, return the base client of manager
	if controllerName == "" {
		return mgr.GetClient()
	}

	clientStore.lock.Lock()
	defer clientStore.lock.Unlock()

	if cli, ok := clientStore.clientsByName[controllerName]; ok {
		return cli
	}

	// check if controller-specific ServiceAccount exist
	_, err := getOrCreateServiceAccount(mgr.GetClient(), namespace, controllerName)
	if err != nil {
		return nil
	}

	// get base config
	baseCfg := mgr.GetConfig()

	// rename cfg user-agent
	cfg := rest.CopyConfig(baseCfg)
	rest.AddUserAgent(cfg, controllerName)

	// add controller-specific token wrapper to cfg
	cachedTokenSource := transport.NewCachedTokenSource(&tokenSourceImpl{
		namespace:          namespace,
		serviceAccountName: controllerName,
		cli:                mgr.GetClient(),
		expirationSeconds:  defaultExpirationSeconds,
		leewayPercent:      defaultLeewayPercent,
	})
	cfg.Wrap(transport.ResettableTokenSourceWrapTransport(cachedTokenSource))

	// construct client from cfg
	clientOptions := client.Options{
		Scheme: mgr.GetScheme(),
		Mapper: mgr.GetRESTMapper(),
		// todo: this is just a default option, we should use mgr's cache options
		Cache: &client.CacheOptions{
			Unstructured: false,
			Reader:       mgr.GetCache(),
		},
	}

	cli, err := client.New(cfg, clientOptions)
	if err != nil {
		panic(err)
	}
	clientStore.clientsByName[controllerName] = cli

	return cli
}

var (
	// defaultExpirationSeconds defines the duration of a TokenRequest in seconds.
	defaultExpirationSeconds = int64(3600)
	// defaultLeewayPercent defines the percentage of expiration left before the client trigger a token rotation.
	// range[0, 100]
	defaultLeewayPercent = 20
)

// migrate from kubernetes/staging/src/k8s.io/controller-manager/pkg/clientbuilder/client_builder_dynamic.go
// change client to controller-runtime client
type tokenSourceImpl struct {
	namespace          string
	serviceAccountName string
	cli                client.Client
	expirationSeconds  int64
	leewayPercent      int
}

func (ts *tokenSourceImpl) Token() (*oauth2.Token, error) {
	retTokenRequest := &v1authenticationapi.TokenRequest{
		Spec: v1authenticationapi.TokenRequestSpec{
			ExpirationSeconds: utilpointer.Int64Ptr(ts.expirationSeconds),
		},
	}

	backoff := wait.Backoff{
		Duration: 500 * time.Millisecond,
		Factor:   2, // double the timeout for every failure
		Steps:    4,
	}
	if err := wait.ExponentialBackoff(backoff, func() (bool, error) {
		sa, inErr := getOrCreateServiceAccount(ts.cli, ts.namespace, ts.serviceAccountName)
		if inErr != nil {
			klog.Warningf("get or create service account failed: %v", inErr)
			return false, nil
		}

		if inErr = ts.cli.SubResource("token").Create(context.Background(), sa, retTokenRequest); inErr != nil {
			klog.Warningf("get token failed: %v", inErr)
			return false, nil
		}

		return true, nil
	}); err != nil {
		return nil, fmt.Errorf("failed to get token for %s/%s: %v", ts.namespace, ts.serviceAccountName, err)
	}

	if retTokenRequest.Spec.ExpirationSeconds == nil {
		return nil, fmt.Errorf("nil pointer of expiration in token request")
	}

	lifetime := retTokenRequest.Status.ExpirationTimestamp.Time.Sub(time.Now())
	if lifetime < time.Minute*10 {
		// possible clock skew issue, pin to minimum token lifetime
		lifetime = time.Minute * 10
	}

	leeway := time.Duration(int64(lifetime) * int64(ts.leewayPercent) / 100)
	expiry := time.Now().Add(lifetime).Add(-1 * leeway)

	return &oauth2.Token{
		AccessToken: retTokenRequest.Status.Token,
		TokenType:   "Bearer",
		Expiry:      expiry,
	}, nil
}

func getOrCreateServiceAccount(cli client.Client, namespace, name string) (*v1.ServiceAccount, error) {
	sa := &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	err := cli.Get(context.TODO(), client.ObjectKey{Namespace: namespace, Name: name}, sa)
	if err == nil {
		return sa, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, err
	}

	// Create the namespace if we can't verify it exists.
	// Tolerate errors, since we don't know whether this component has namespace creation permissions.
	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	if err := cli.Get(context.TODO(), client.ObjectKey{}, ns); apierrors.IsNotFound(err) {
		if err = cli.Create(context.TODO(), ns); err != nil && !apierrors.IsAlreadyExists(err) {
			klog.Warningf("create non-exist namespace %s failed:%v", namespace, err)
		}
	}

	// Create the service account
	err = cli.Create(context.TODO(), sa)
	if apierrors.IsAlreadyExists(err) {
		// If we're racing to init and someone else already created it, re-fetch
		err = cli.Get(context.TODO(), client.ObjectKey{Namespace: namespace, Name: name}, sa)
		return sa, err
	}
	return sa, err
}

```

If you are developing a new controller/webhook in yurt-manager, you should use `GetClientByControllerNameOrDie(mgr, controllerName)` instead of `mgr.GetClient()` to apply your own RBAC.

## Implementation History

* [ ]  05/17/2024: Draft proposal created;
