#!/usr/bin/env bash

# Copyright 2023 The OpenYurt Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# exit immediately when a command fails
#set -e
# only exit with zero if all commands of the pipeline exit successfully
#set -o pipefail
# error on unset variables
#set -u

#set -x

SCOPE_NAMESPACE="Namespaced"
SCOPE_CLUSTER="Cluster"
TRUE_FLAG="true"
FALSE_FLAG="false"

function usage(){
    echo "$0 [Options]"
    echo -e "Options:"
    echo -e "\t-g, --group\t crd group name. such as [apps]"
    echo -e "\t-v, --version\t crd version name. such as[v1beta1]"
    echo -e "\t-k, --kind\t crd kind name. It must be singular, such as [Sample]"
    echo -e "\t-sn, --shortname\t crd kind short name. such as [s]"
    echo -e "\t-s, --scope\t crd scoped , support [${SCOPE_NAMESPACE} ${SCOPE_CLUSTER}]."
    echo -e "\t-sp, --special\t support webhook to handle AdmissionRequest."
    exit 1
}

YURT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"

while [ $# -gt 0 ];do
    case $1 in
    --group|-g)
      shift
      GROUP=$1
      shift
      ;;
    --version|-v)
      shift
      VERSION=$1
      shift
      ;;
    --kind|-k)
      shift
      KIND=$1
      shift
      ;;
    --scope|-s)
      shift
      SCOPE=$1
      shift
      ;;
    --shortname|-sn)
      shift
      SHORTNAME=$1
      shift
      ;;
    --special|-sp)
      shift
      SPECIAL=true
      shift
      ;;

    --help|-h)
      shift
      usage
      ;;
    *)
      usage
      ;;
    esac
done

if [ -z $GROUP ] || [ -z $VERSION ] || [ -z $KIND ] || [ -z $SCOPE ] || [ -z $SHORTNAME ] ; then
    usage
fi

# support bash 3 [mac and linux]
# @kadisi
# Make letters lowercase
GROUP=$(echo $GROUP | tr '[A-Z]' '[a-z]')
VERSION=$(echo $VERSION | tr '[A-Z]' '[a-z]')
SHORTNAME=$(echo $SHORTNAME | tr '[A-Z]' '[a-z]')

#KIND=$(echo $KIND | tr '[A-Z]' '[a-z]')

# support bash 3 [mac and linux]
# @kadisi
KIND_INITIAL_UPPER=$(echo ${KIND: 0:1} | tr '[a-z]' '[A-Z]')
KIND_INITIAL_LOWER=$(echo ${KIND: 0:1} | tr '[A-Z]' '[a-z]')
KIND_ALL_LOWER=$(echo ${KIND} | tr '[A-Z]' '[a-z]')

KIND_PLURAL="${KIND_ALL_LOWER}s"
KIND_FIRST_UPPER=${KIND_INITIAL_UPPER}${KIND: 1}

KIND_UPPER_SHORTNAME=$(echo ${KIND_FIRST_UPPER} | grep -o '[A-Z]' | tr -d '\n')
KIND_LOWER_SHORTNAME=$(echo ${KIND_UPPER_SHORTNAME} | tr '[A-Z]' '[a-z]')

echo "Add controller Group: $GROUP Version: $VERSION Instance Kind: $KIND_FIRST_UPPER ShortName: $SHORTNAME"

if [ $SCOPE != $SCOPE_NAMESPACE ] && [ $SCOPE != $SCOPE_CLUSTER ]; then
    echo "scope only support [$SCOPE_NAMESPACE $SCOPE_CLUSTER]"
    exit 1
fi

CMD_YURT_MANGER_DIR=${YURT_ROOT}/cmd/yurt-manager
PKG_DIR=${YURT_ROOT}/pkg/yurtmanager
APIS_DIR=${YURT_ROOT}/pkg/apis
CONTROLLER_DIR=${PKG_DIR}/controller
WEBHOOK_DIR=${PKG_DIR}/webhook

if [ ! -d ${PKG_DIR} ] || [ ! -d ${APIS_DIR} ] || [ ! -d ${CONTROLLER_DIR} ] || [ ! -d ${WEBHOOK_DIR} ] ; then
    echo "Please check pkg、apis、controller、webhook dir ..."
    exit 1
fi

APP_OPTION_KIND_CONTROLLER_FILE=${CMD_YURT_MANGER_DIR}/app/options/${KIND_ALL_LOWER}controller.go

CRD_GROUP_DIR=${APIS_DIR}/${GROUP}
CRD_GROUP_VERSION_DIR=${CRD_GROUP_DIR}/${VERSION}

CRD_GROUP_VERSION_KIND_FILE=${CRD_GROUP_VERSION_DIR}/${KIND_ALL_LOWER}_types.go
CRD_GROUP_VERSION_DEFAULT_FILE=${CRD_GROUP_VERSION_DIR}/default.go

KIND_CONTROLLER_DIR=${CONTROLLER_DIR}/${KIND_ALL_LOWER}
KIND_CONTROLLER_CONFIG_DIR=${KIND_CONTROLLER_DIR}/config

KIND_CONTROLLER_CONFIG_TYPE_FILE=${KIND_CONTROLLER_CONFIG_DIR}/types.go
KIND_CONTROLLER_FILE=${KIND_CONTROLLER_DIR}/${KIND_ALL_LOWER}_controller.go

WEBHOOK_KIND_DIR=${WEBHOOK_DIR}/${KIND_ALL_LOWER}

WEBHOOK_KIND_VERSION_DIR=${WEBHOOK_KIND_DIR}/${VERSION}

WEBHOOK_KIND_VERSION_DEFAULT_FILE=${WEBHOOK_KIND_VERSION_DIR}/${KIND_ALL_LOWER}_default.go
WEBHOOK_KIND_VERSION_HANDLE_FILE=${WEBHOOK_KIND_VERSION_DIR}/${KIND_ALL_LOWER}_handler.go
WEBHOOK_KIND_VERSION_VALIDATION_FILE=${WEBHOOK_KIND_VERSION_DIR}/${KIND_ALL_LOWER}_validation.go

if [ -f "${APP_OPTION_KIND_CONTROLLER_FILE}" ]; then
    echo "options controller file [${APP_OPTION_KIND_CONTROLLER_FILE}] already exist ..."
    exit 1
fi

if [ -f "${CRD_GROUP_VERSION_KIND_FILE}" ]; then
    echo "Instance crd[${GROUP}/${VERSION}/${KIND_ALL_LOWER}] already exist ..."
    exit 1
fi

if [ -d "${KIND_CONTROLLER_DIR}" ]; then
    echo "instance controller dir ${KIND_CONTROLLER_DIR} already exist ..."
    exit 1
fi

if [ -d "${WEBHOOK_KIND_DIR}" ]; then
    echo "instance webhook dir ${WEBHOOK_KIND_DIR} already exist ..."
    exit 1
fi

function create_header() {
    local packageName=$1
    echo "/*
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

package ${packageName}
"
}

function build_new_version_frame() {
    local doc_file=$CRD_GROUP_VERSION_DIR/doc.go
    local group_version_info_file=$CRD_GROUP_VERSION_DIR/groupversion_info.go

    mkdir -p ${CRD_GROUP_VERSION_DIR}

    cat > $doc_file <<EOF
$(create_header ${VERSION})
EOF

    cat > $group_version_info_file <<EOF
$(create_header ${VERSION})

// Package ${VERSION} contains API Schema definitions for the ${GROUP} ${VERSION}API group
// +kubebuilder:object:generate=true
// +groupName=${GROUP}.openyurt.io

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	// GroupVersion is group version used to register these objects
	GroupVersion = schema.GroupVersion{Group: "${GROUP}.openyurt.io", Version: "${VERSION}"}

	SchemeGroupVersion = GroupVersion

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

// Resource is required by pkg/client/listers/...
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

EOF

    cat > $CRD_GROUP_VERSION_DEFAULT_FILE <<EOF
$(create_header ${VERSION})
EOF

}

function create_addtoscheme_group_version_file() {
    local addtoscheme_group_version_file=$1
    if [ -f $addtoscheme_group_version_file ]; then
        return
    fi
    cat > ${addtoscheme_group_version_file} <<EOF
$(create_header apis)

import (
	version "github.com/openyurtio/openyurt/pkg/apis/${GROUP}/${VERSION}"
)

func init() {
	// Register the types with the Scheme so the components can map objects to GroupVersionKinds and back
	AddToSchemes = append(AddToSchemes, version.SchemeBuilder.AddToScheme)
}
EOF
}


function build_apis_frame() {
    local need_create_version_dir="False"

    if [ ! -d ${CRD_GROUP_DIR} ]; then
        echo "Group ${GROUP} not exist, create it."
        mkdir -p ${CRD_GROUP_DIR}
    fi

    if [ ! -d ${CRD_GROUP_VERSION_DIR} ]; then
        echo "Version ${VERSION} not exist, create it."
        build_new_version_frame
        need_create_version_dir="True"
    fi

    local addtoscheme_group_version_file=${APIS_DIR}/addtoscheme_${GROUP}_${VERSION}.go

    if [ $need_create_version_dir="True" ]; then
        create_addtoscheme_group_version_file $addtoscheme_group_version_file
    fi

    cat > ${CRD_GROUP_VERSION_KIND_FILE} << EOF
$(create_header ${VERSION})

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ${KIND_FIRST_UPPER}Spec defines the desired state of ${KIND_FIRST_UPPER}
type ${KIND_FIRST_UPPER}Spec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of ${KIND_FIRST_UPPER}. Edit sample_types.go to remove/update
	Foo string \`json:"foo,omitempty"\`

	// Default is an example field of ${KIND_FIRST_UPPER}. Edit sample_types.go to remove/update
	Default string \`json:"default,omitempty"\`
}

// ${KIND_FIRST_UPPER}Status defines the observed state of ${KIND_FIRST_UPPER}
type ${KIND_FIRST_UPPER}Status struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of ${KIND_FIRST_UPPER}. Edit sample_types.go to remove/update
	Foo string \`json:"foo,omitempty"\`

	// Default is an example field of ${KIND_FIRST_UPPER}. Edit sample_types.go to remove/update
	Default string \`json:"default,omitempty"\`
}


// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=${SCOPE},path=${KIND_PLURAL},shortName=${SHORTNAME},categories=all
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp",description="CreationTimestamp is a timestamp representing the server time when this object was created. It is not guaranteed to be set in happens-before order across separate operations. Clients may not set this value. It is represented in RFC3339 form and is in UTC."

// ${KIND_FIRST_UPPER} is the Schema for the samples API
type ${KIND_FIRST_UPPER} struct {
	metav1.TypeMeta   \`json:",inline"\`
	metav1.ObjectMeta \`json:"metadata,omitempty"\`

	Spec   ${KIND_FIRST_UPPER}Spec   \`json:"spec,omitempty"\`
	Status ${KIND_FIRST_UPPER}Status \`json:"status,omitempty"\`
}


//+kubebuilder:object:root=true

// ${KIND_FIRST_UPPER}List contains a list of ${KIND_FIRST_UPPER}
type ${KIND_FIRST_UPPER}List struct {
	metav1.TypeMeta \`json:",inline"\`
	metav1.ListMeta \`json:"metadata,omitempty"\`
	Items           []${KIND_FIRST_UPPER} \`json:"items"\`
}

func init() {
	SchemeBuilder.Register(&${KIND_FIRST_UPPER}{}, &${KIND_FIRST_UPPER}List{})
}
EOF

    # append version_default file
    cat >> $CRD_GROUP_VERSION_DEFAULT_FILE << EOF

// SetDefaults${KIND_FIRST_UPPER} set default values for ${KIND_FIRST_UPPER}.
func SetDefaults${KIND_FIRST_UPPER}(obj *${KIND_FIRST_UPPER}) {
	// example for set default value for ${KIND_FIRST_UPPER}

	if len(obj.Spec.Default) == 0 {
		obj.Spec.Default = "set-default-value-0"
	}
}
EOF
}


function build_controller_frame() {
    # create instance controller
    mkdir -p ${KIND_CONTROLLER_DIR}
    mkdir -p ${KIND_CONTROLLER_CONFIG_DIR}

    cat > ${KIND_CONTROLLER_CONFIG_TYPE_FILE} << EOF
$(create_header config)

// ${KIND_FIRST_UPPER}ControllerConfiguration contains elements describing ${KIND_FIRST_UPPER}Controller.
type ${KIND_FIRST_UPPER}ControllerConfiguration struct {
}

EOF

    # create controller file
    cat > ${KIND_CONTROLLER_FILE} <<EOF
$(create_header ${KIND_ALL_LOWER})

import (
	"context"
	"flag"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	${GROUP}${VERSION} "github.com/openyurtio/openyurt/pkg/apis/${GROUP}/${VERSION}"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/${KIND_ALL_LOWER}/config"
)

func init() {
	flag.IntVar(&concurrentReconciles, "${KIND_ALL_LOWER}-workers", concurrentReconciles, "Max concurrent workers for ${KIND_FIRST_UPPER} controller.")
}

var (
	concurrentReconciles = 3
	controllerKind       = ${GROUP}${VERSION}.SchemeGroupVersion.WithKind("${KIND_FIRST_UPPER}")
)

const (
	controllerName = names.xxx
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", controllerName, s)
}

// Add creates a new ${KIND_FIRST_UPPER} Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(ctx context.Context, c *appconfig.CompletedConfig, mgr manager.Manager) error {
	klog.Info(Format("${KIND_ALL_LOWER}-controller add controller %s", controllerKind.String()))
	return add(mgr, newReconciler(c, mgr))
}

var _ reconcile.Reconciler = &Reconcile${KIND_FIRST_UPPER}{}

// Reconcile${KIND_FIRST_UPPER} reconciles a ${KIND_FIRST_UPPER} object
type Reconcile${KIND_FIRST_UPPER} struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
	Configuration config.${KIND_FIRST_UPPER}ControllerConfiguration
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(c *appconfig.CompletedConfig, mgr manager.Manager) reconcile.Reconciler {
	return &Reconcile${KIND_FIRST_UPPER}{
		Client:   mgr.GetClient(),
		scheme:   mgr.GetScheme(),
		recorder: mgr.GetEventRecorderFor(controllerName),
        Configuration: c.ComponentConfig.${KIND_FIRST_UPPER}Controller,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: concurrentReconciles,
	})
	if err != nil {
		return err
	}

	// Watch for changes to ${KIND_FIRST_UPPER}
	err = c.Watch(&source.Kind{Type: &${GROUP}${VERSION}.${KIND_FIRST_UPPER}{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

// +kubebuilder:rbac:groups=${GROUP}.openyurt.io,resources=${KIND_PLURAL},verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=${GROUP}.openyurt.io,resources=${KIND_PLURAL}/status,verbs=get;update;patch

// Reconcile reads that state of the cluster for a ${KIND_FIRST_UPPER} object and makes changes based on the state read
// and what is in the ${KIND_FIRST_UPPER}.Spec
func (r *Reconcile${KIND_FIRST_UPPER}) Reconcile(_ context.Context, request reconcile.Request) (reconcile.Result, error) {

	// Note !!!!!!!!!!
	// We strongly recommend use Format() to  encapsulation because Format() can print logs by module
	// @kadisi
	klog.Info(Format("Reconcile ${KIND_FIRST_UPPER} %s/%s", request.Namespace, request.Name))

	// Fetch the ${KIND_FIRST_UPPER} instance
	instance := &${GROUP}${VERSION}.${KIND_FIRST_UPPER}{}
	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	if instance.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	// Update Status
	if instance.Spec.Foo != instance.Status.Foo {
		instance.Status.Foo = instance.Spec.Foo
		if err = r.Status().Update(context.TODO(), instance); err != nil {
			klog.Error(Format("Update ${KIND_FIRST_UPPER} Status %s error %v", klog.KObj(instance), err))
			return reconcile.Result{Requeue: true}, err
		}
	}

	// Update Instance
	//if err = r.Update(context.TODO(), instance); err != nil {
	//	klog.Error(Format("Update ${KIND_FIRST_UPPER} %s error %v", klog.KObj(instance), err))
	//	return reconcile.Result{Requeue: true}, err
	//}

	return reconcile.Result{}, nil
}
EOF

}


function build_webhook_frame() {

    if [ -d ${WEBHOOK_KIND_VERSION_DIR} ]; then
        echo "${WEBHOOK_KIND_VERSION_DIR} dir has exist ..."
        exit 1
    fi

    mkdir -p ${WEBHOOK_KIND_VERSION_DIR}

   cat > $WEBHOOK_KIND_VERSION_DEFAULT_FILE << EOF
$(create_header ${VERSION})

import (
	"context"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openyurtio/openyurt/pkg/apis/${GROUP}/${VERSION}"
)


// Default satisfies the defaulting webhook interface.
func (webhook *${KIND_FIRST_UPPER}Handler) Default(ctx context.Context, obj runtime.Object) error {
	${KIND_LOWER_SHORTNAME}, ok := obj.(*${VERSION}.${KIND_FIRST_UPPER})
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a ${KIND_FIRST_UPPER} but got a %T", obj))
	}

	${VERSION}.SetDefaults${KIND_FIRST_UPPER}(${KIND_LOWER_SHORTNAME})

	return nil
}

EOF

    gofmt -w ${WEBHOOK_KIND_VERSION_DEFAULT_FILE}
    goimports -w ${WEBHOOK_KIND_VERSION_DEFAULT_FILE}

    cat > ${WEBHOOK_KIND_VERSION_HANDLE_FILE} << EOF
$(create_header ${VERSION})


import (
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/openyurtio/openyurt/pkg/yurtmanager/webhook/util"
	"github.com/openyurtio/openyurt/pkg/apis/${GROUP}/${VERSION}"
)


// SetupWebhookWithManager sets up Cluster webhooks. 	mutate path, validatepath, error
func (webhook *${KIND_FIRST_UPPER}Handler) SetupWebhookWithManager(mgr ctrl.Manager) (string, string, error) {
	// init
	webhook.Client = mgr.GetClient()

	gvk, err := apiutil.GVKForObject(&${VERSION}.${KIND_FIRST_UPPER}{}, mgr.GetScheme())
	if err != nil {
		return "", "", err
	}
	return util.GenerateMutatePath(gvk),
		util.GenerateValidatePath(gvk),
		ctrl.NewWebhookManagedBy(mgr).
			For(&${VERSION}.${KIND_FIRST_UPPER}{}).
			WithDefaulter(webhook).
			WithValidator(webhook).
			Complete()
}


// +kubebuilder:webhook:path=/validate-${GROUP}-openyurt-io-${KIND_ALL_LOWER},mutating=false,failurePolicy=fail,sideEffects=None,admissionReviewVersions=v1;v1beta1,groups=${GROUP}.openyurt.io,resources=${KIND_PLURAL},verbs=create;update,versions=${VERSION},name=validate.${GROUP}.${VERSION}.${KIND_ALL_LOWER}.openyurt.io
// +kubebuilder:webhook:path=/mutate-${GROUP}-openyurt-io-${KIND_ALL_LOWER},mutating=true,failurePolicy=fail,sideEffects=None,admissionReviewVersions=v1;v1beta1,groups=${GROUP}.openyurt.io,resources=${KIND_PLURAL},verbs=create;update,versions=${VERSION},name=mutate.${GROUP}.${VERSION}.${KIND_ALL_LOWER}.openyurt.io


// Cluster implements a validating and defaulting webhook for Cluster.
type ${KIND_FIRST_UPPER}Handler struct {
	Client client.Client
}

var _ webhook.CustomDefaulter = &${KIND_FIRST_UPPER}Handler{}
var _ webhook.CustomValidator = &${KIND_FIRST_UPPER}Handler{}

EOF
    gofmt -w ${WEBHOOK_KIND_VERSION_HANDLE_FILE}
    goimports -w ${WEBHOOK_KIND_VERSION_HANDLE_FILE}

    cat > ${WEBHOOK_KIND_VERSION_VALIDATION_FILE} <<EOF
$(create_header ${VERSION})

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apivalidation "k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openyurtio/openyurt/pkg/apis/${GROUP}/${VERSION}"
)

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *${KIND_FIRST_UPPER}Handler) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	${KIND_LOWER_SHORTNAME}, ok := obj.(*${VERSION}.${KIND_FIRST_UPPER})
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a ${KIND_FIRST_UPPER} but got a %T", obj))
	}

	//validate

	return nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *${KIND_FIRST_UPPER}Handler) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) error {
	new${KIND_LOWER_SHORTNAME}, ok := newObj.(*${VERSION}.${KIND_FIRST_UPPER})
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a ${KIND_FIRST_UPPER} but got a %T", newObj))
	}
	old${KIND_LOWER_SHORTNAME}, ok := oldObj.(*${VERSION}.${KIND_FIRST_UPPER})
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a ${KIND_FIRST_UPPER}} but got a %T", oldObj))
	}

	// validate
	return nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *${KIND_FIRST_UPPER}Handler) ValidateDelete(_ context.Context, obj runtime.Object) error {
	${KIND_LOWER_SHORTNAME}, ok := obj.(*${VERSION}.${KIND_FIRST_UPPER})
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a ${KIND_FIRST_UPPER} but got a %T", obj))
	}

	// validate
	return nil
}
EOF

    gofmt -w ${WEBHOOK_KIND_VERSION_VALIDATION_FILE}
    goimports -w ${WEBHOOK_KIND_VERSION_VALIDATION_FILE}

}

function build_options() {
    cat > ${APP_OPTION_KIND_CONTROLLER_FILE} << EOF
$(create_header options)

import (
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/${KIND_ALL_LOWER}/config"
	"github.com/spf13/pflag"
)

type ${KIND_FIRST_UPPER}ControllerOptions struct {
	*config.${KIND_FIRST_UPPER}ControllerConfiguration
}

func New${KIND_FIRST_UPPER}ControllerOptions() *${KIND_FIRST_UPPER}ControllerOptions {
	return &${KIND_FIRST_UPPER}ControllerOptions{
		&config.${KIND_FIRST_UPPER}ControllerConfiguration{
		},
	}
}

// AddFlags adds flags related to ${KIND_ALL_LOWER} for yurt-manager to the specified FlagSet.
func (n *${KIND_FIRST_UPPER}ControllerOptions) AddFlags(fs *pflag.FlagSet) {
	if n == nil {
		return
	}

	//fs.BoolVar(&n.CreateDefaultPool, "create-default-pool", n.CreateDefaultPool, "Create default cloud/edge pools if indicated.")
}

// ApplyTo fills up ${KIND_ALL_LOWER} config with options.
func (o *${KIND_FIRST_UPPER}ControllerOptions) ApplyTo(cfg *config.${KIND_FIRST_UPPER}ControllerConfiguration) error {
	if o == nil {
		return nil
	}

	return nil
}

// Validate checks validation of ${KIND_FIRST_UPPER}ControllerOptions.
func (o *${KIND_FIRST_UPPER}ControllerOptions) Validate() []error {
	if o == nil {
		return nil
	}
	errs := []error{}
	return errs
}
EOF
}

function build_webhook_special_frame() {

    if [ -d ${WEBHOOK_KIND_VERSION_DIR} ]; then
        echo "${WEBHOOK_KIND_VERSION_DIR} dir has exist ..."
        exit 1
    fi

    mkdir -p ${WEBHOOK_KIND_VERSION_DIR}

   cat > $WEBHOOK_KIND_VERSION_DEFAULT_FILE << EOF
$(create_header ${VERSION})

import (
	"context"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/openyurtio/openyurt/pkg/apis/${GROUP}/${VERSION}"
)


// Default satisfies the defaulting webhook interface.
func (webhook *${KIND_FIRST_UPPER}Handler) Default(ctx context.Context, obj runtime.Object, req admission.Request) error {
	${KIND_LOWER_SHORTNAME}, ok := obj.(*${VERSION}.${KIND_FIRST_UPPER})
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a ${KIND_FIRST_UPPER} but got a %T", obj))
	}

	${VERSION}.SetDefaults${KIND_FIRST_UPPER}(${KIND_LOWER_SHORTNAME})

	return nil
}

EOF

    gofmt -w ${WEBHOOK_KIND_VERSION_DEFAULT_FILE}
    goimports -w ${WEBHOOK_KIND_VERSION_DEFAULT_FILE}

    cat > ${WEBHOOK_KIND_VERSION_HANDLE_FILE} << EOF
$(create_header ${VERSION})


import (
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	"github.com/openyurtio/openyurt/pkg/yurtmanager/webhook/builder"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/webhook/util"
	"github.com/openyurtio/openyurt/pkg/apis/${GROUP}/${VERSION}"
)


// SetupWebhookWithManager sets up Cluster webhooks. 	mutate path, validatepath, error
func (webhook *${KIND_FIRST_UPPER}Handler) SetupWebhookWithManager(mgr ctrl.Manager) (string, string, error) {
	// init
	webhook.Client = mgr.GetClient()

	gvk, err := apiutil.GVKForObject(&${VERSION}.${KIND_FIRST_UPPER}{}, mgr.GetScheme())
	if err != nil {
		return "", "", err
	}
	return util.GenerateMutatePath(gvk),
		util.GenerateValidatePath(gvk),
		builder.WebhookManagedBy(mgr).
			For(&${VERSION}.${KIND_FIRST_UPPER}{}).
			WithDefaulter(webhook).
			WithValidator(webhook).
			Complete()
}


// +kubebuilder:webhook:path=/validate-${GROUP}-openyurt-io-${KIND_ALL_LOWER},mutating=false,failurePolicy=fail,sideEffects=None,admissionReviewVersions=v1;v1beta1,groups=${GROUP}.openyurt.io,resources=${KIND_PLURAL},verbs=create;update,versions=${VERSION},name=validate.${GROUP}.${VERSION}.${KIND_ALL_LOWER}.openyurt.io
// +kubebuilder:webhook:path=/mutate-${GROUP}-openyurt-io-${KIND_ALL_LOWER},mutating=true,failurePolicy=fail,sideEffects=None,admissionReviewVersions=v1;v1beta1,groups=${GROUP}.openyurt.io,resources=${KIND_PLURAL},verbs=create;update,versions=${VERSION},name=mutate.${GROUP}.${VERSION}.${KIND_ALL_LOWER}.openyurt.io


// Cluster implements a validating and defaulting webhook for Cluster.
type ${KIND_FIRST_UPPER}Handler struct {
	Client client.Client
}

var _ builder.CustomDefaulter = &${KIND_FIRST_UPPER}Handler{}
var _ builder.CustomValidator = &${KIND_FIRST_UPPER}Handler{}

EOF
    gofmt -w ${WEBHOOK_KIND_VERSION_HANDLE_FILE}
    goimports -w ${WEBHOOK_KIND_VERSION_HANDLE_FILE}

    cat > ${WEBHOOK_KIND_VERSION_VALIDATION_FILE} <<EOF
$(create_header ${VERSION})

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apivalidation "k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/openyurtio/openyurt/pkg/apis/${GROUP}/${VERSION}"
)

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *${KIND_FIRST_UPPER}Handler) ValidateCreate(ctx context.Context, obj runtime.Object, req admission.Request) error {
	${KIND_LOWER_SHORTNAME}, ok := obj.(*${VERSION}.${KIND_FIRST_UPPER})
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a ${KIND_FIRST_UPPER} but got a %T", obj))
	}

	//validate
	return nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *${KIND_FIRST_UPPER}Handler) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object, req admission.Request) error {
	new${KIND_LOWER_SHORTNAME}, ok := newObj.(*${VERSION}.${KIND_FIRST_UPPER})
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a ${KIND_FIRST_UPPER} but got a %T", newObj))
	}
	old${KIND_LOWER_SHORTNAME}, ok := oldObj.(*${VERSION}.${KIND_FIRST_UPPER})
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a ${KIND_FIRST_UPPER}} but got a %T", oldObj))
	}

	// validate
	return nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *${KIND_FIRST_UPPER}Handler) ValidateDelete(_ context.Context, obj runtime.Object, req admission.Request) error {
	${KIND_LOWER_SHORTNAME}, ok := obj.(*${VERSION}.${KIND_FIRST_UPPER})
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a ${KIND_FIRST_UPPER} but got a %T", obj))
	}

	// validate
	return nil
}
EOF

    gofmt -w ${WEBHOOK_KIND_VERSION_VALIDATION_FILE}
    goimports -w ${WEBHOOK_KIND_VERSION_VALIDATION_FILE}

}

build_options
build_apis_frame
build_controller_frame
if [ $SPECIAL ]; then
  build_webhook_special_frame
else
  build_webhook_frame
fi
