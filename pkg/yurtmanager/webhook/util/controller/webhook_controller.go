/*
Copyright 2023 The OpenYurt Authors.

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

package controller

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	v1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apiextensionsinformers "k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions"
	apiextensionslister "k8s.io/apiextensions-apiserver/pkg/client/listers/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	admissionregistrationinformers "k8s.io/client-go/informers/admissionregistration/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/pkg/apis"
	webhookutil "github.com/openyurtio/openyurt/pkg/yurtmanager/webhook/util"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/webhook/util/configuration"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/webhook/util/generator"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/webhook/util/writer"
)

const (
	defaultResyncPeriod = time.Minute
)

var (
	uninit   = make(chan struct{})
	onceInit = sync.Once{}

	yurtScheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(apis.AddToScheme(yurtScheme))
}

func Inited() chan struct{} {
	return uninit
}

type Controller struct {
	kubeClient       clientset.Interface
	extensionsLister apiextensionslister.CustomResourceDefinitionLister
	extensionsClient apiextensionsclientset.Interface
	handlers         map[string]struct{}

	informerFactory           informers.SharedInformerFactory
	extensionsInformerFactory apiextensionsinformers.SharedInformerFactory
	synced                    []cache.InformerSynced

	queue       workqueue.RateLimitingInterface
	webhookPort int
}

func New(handlers map[string]struct{}, cc *config.CompletedConfig, restCfg *rest.Config) (*Controller, error) {
	kubeClient, err := clientset.NewForConfig(restCfg)
	if err != nil {
		return nil, err
	}
	c := &Controller{
		kubeClient:  kubeClient,
		handlers:    handlers,
		queue:       workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "webhook-controller"),
		webhookPort: cc.ComponentConfig.Generic.WebhookPort,
	}

	c.informerFactory = informers.NewSharedInformerFactory(c.kubeClient, 0)
	admissionRegistrationInformer := admissionregistrationinformers.New(c.informerFactory, v1.NamespaceAll, nil)

	extensionsClient, err := apiextensionsclientset.NewForConfig(restCfg)
	if err != nil {
		return nil, err
	}
	apiExtensionsInformerFactory := apiextensionsinformers.NewSharedInformerFactory(extensionsClient, 0)
	c.extensionsInformerFactory = apiExtensionsInformerFactory
	crdInformer := apiExtensionsInformerFactory.Apiextensions().V1().CustomResourceDefinitions()
	crdInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			crd := obj.(*apiextensionsv1.CustomResourceDefinition)
			if yurtCRDHasWebhookConversion(crd) {
				klog.Infof("CRD %s with conversion added", crd.Name)
				c.queue.Add(crd.Name)
			}
		},
		UpdateFunc: func(old, new interface{}) {
			crd := new.(*apiextensionsv1.CustomResourceDefinition)
			if yurtCRDHasWebhookConversion(crd) {
				klog.Infof("CRD %s with conversion updated", crd.Name)
				c.queue.Add(crd.Name)
			}
		},
	})
	c.extensionsClient = extensionsClient
	c.extensionsLister = crdInformer.Lister()

	admissionRegistrationInformer.MutatingWebhookConfigurations().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			conf := obj.(*admissionregistrationv1.MutatingWebhookConfiguration)
			if conf.Name == webhookutil.MutatingWebhookConfigurationName {
				klog.Infof("MutatingWebhookConfiguration %s added", webhookutil.MutatingWebhookConfigurationName)
				c.queue.Add("")
			}
		},
		UpdateFunc: func(old, cur interface{}) {
			conf := cur.(*admissionregistrationv1.MutatingWebhookConfiguration)
			if conf.Name == webhookutil.MutatingWebhookConfigurationName {
				klog.Infof("MutatingWebhookConfiguration %s update", webhookutil.MutatingWebhookConfigurationName)
				c.queue.Add("")
			}
		},
	})

	admissionRegistrationInformer.ValidatingWebhookConfigurations().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			conf := obj.(*admissionregistrationv1.ValidatingWebhookConfiguration)
			if conf.Name == webhookutil.ValidatingWebhookConfigurationName {
				klog.Infof("ValidatingWebhookConfiguration %s added", webhookutil.ValidatingWebhookConfigurationName)
				c.queue.Add("")
			}
		},
		UpdateFunc: func(old, cur interface{}) {
			conf := cur.(*admissionregistrationv1.ValidatingWebhookConfiguration)
			if conf.Name == webhookutil.ValidatingWebhookConfigurationName {
				klog.Infof("ValidatingWebhookConfiguration %s updated", webhookutil.ValidatingWebhookConfigurationName)
				c.queue.Add("")
			}
		},
	})

	c.synced = []cache.InformerSynced{
		admissionRegistrationInformer.MutatingWebhookConfigurations().Informer().HasSynced,
		admissionRegistrationInformer.ValidatingWebhookConfigurations().Informer().HasSynced,
		crdInformer.Informer().HasSynced,
	}

	return c, nil
}

func (c *Controller) Start(ctx context.Context) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	klog.Infof("Starting webhook-controller")
	defer klog.Infof("Shutting down webhook-controller")

	c.informerFactory.Start(ctx.Done())
	c.extensionsInformerFactory.Start(ctx.Done())
	if !cache.WaitForNamedCacheSync("webhook-controller", ctx.Done(), c.synced...) {
		klog.Errorf("Wait For Cache sync webhook-controller failed")
		return
	}

	go wait.Until(func() {
		for c.processNextWorkItem() {
		}
	}, time.Second, ctx.Done())
	klog.Infof("Started webhook-controller")

	<-ctx.Done()
}

func (c *Controller) processNextWorkItem() bool {
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(key)

	err := c.sync(key.(string))
	if err == nil {
		c.queue.AddAfter(key, defaultResyncPeriod)
		c.queue.Forget(key)
		return true
	}

	utilruntime.HandleError(fmt.Errorf("sync %q failed with %v", key, err))
	c.queue.AddRateLimited(key)

	return true
}

func (c *Controller) sync(key string) error {
	klog.V(5).Infof("Starting to sync webhook certs and configurations")
	defer func() {
		klog.V(5).Infof("Finished to sync webhook certs and configurations")
	}()

	dnsName := webhookutil.GetHost()
	if len(dnsName) == 0 {
		dnsName = generator.ServiceToCommonName(webhookutil.GetNamespace(), webhookutil.GetServiceName())
	}

	var certWriter writer.CertWriter
	var err error

	certWriterType := webhookutil.GetCertWriter()
	if certWriterType == writer.FsCertWriter || (len(certWriterType) == 0 && len(webhookutil.GetHost()) != 0) {
		certWriter, err = writer.NewFSCertWriter(writer.FSCertWriterOptions{
			Path: webhookutil.GetCertDir(),
		})
	} else {
		certWriter, err = writer.NewSecretCertWriter(writer.SecretCertWriterOptions{
			Clientset: c.kubeClient,
			Secret:    &types.NamespacedName{Namespace: webhookutil.GetNamespace(), Name: webhookutil.GetSecretName()},
		})
	}
	if err != nil {
		return fmt.Errorf("could not ensure certs: %v", err)
	}

	certs, _, err := certWriter.EnsureCert(dnsName)
	if err != nil {
		return fmt.Errorf("could not ensure certs: %v", err)
	}
	if err := writer.WriteCertsToDir(webhookutil.GetCertDir(), certs); err != nil {
		return fmt.Errorf("could not write certs to dir: %v", err)
	}

	if err := configuration.Ensure(c.kubeClient, c.handlers, certs.CACert, c.webhookPort); err != nil {
		return fmt.Errorf("could not ensure configuration: %v", err)
	}

	if len(key) != 0 {
		crd, err := c.extensionsLister.Get(key)
		if err != nil {
			klog.Errorf("could not get crd(%s), %v", key, err)
			return err
		}

		if err := ensureCRDConversionCA(c.extensionsClient, crd, certs.CACert); err != nil {
			klog.Errorf("could not ensure conversion configuration for crd(%s), %v", crd.Name, err)
			return err
		}
	}

	onceInit.Do(func() {
		close(uninit)
	})
	return nil
}

func yurtCRDHasWebhookConversion(crd *apiextensionsv1.CustomResourceDefinition) bool {
	// it is an openyurt crd
	if !strings.Contains(crd.Spec.Group, "openyurt.io") {
		return false
	}

	conversion := crd.Spec.Conversion
	if conversion == nil {
		return false
	}

	if conversion.Strategy == apiextensionsv1.WebhookConverter {
		return true
	}

	return false
}

func ensureCRDConversionCA(client apiextensionsclientset.Interface, crd *apiextensionsv1.CustomResourceDefinition, newCABundle []byte) error {
	if len(crd.Spec.Versions) == 0 ||
		crd.Spec.Conversion == nil ||
		crd.Spec.Conversion.Strategy != apiextensionsv1.WebhookConverter ||
		crd.Spec.Conversion.Webhook == nil ||
		crd.Spec.Conversion.Webhook.ClientConfig == nil {
		return nil
	}

	if !yurtScheme.Recognizes(schema.GroupVersionKind{
		Group:   crd.Spec.Group,
		Version: crd.Spec.Versions[0].Name,
		Kind:    crd.Spec.Names.Kind,
	}) {
		return nil
	}

	if bytes.Equal(crd.Spec.Conversion.Webhook.ClientConfig.CABundle, newCABundle) {
		return nil
	}

	crd.Spec.Conversion.Webhook.ClientConfig.CABundle = newCABundle
	// update crd
	_, err := client.ApiextensionsV1().CustomResourceDefinitions().Update(context.TODO(), crd, metav1.UpdateOptions{})
	return err
}
