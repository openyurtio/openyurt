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

function usage(){
    echo "$0 [Options]"
    echo -e "Options:"
    echo -e "\t-g, --group\t crd group name. such as [apps]"
    echo -e "\t-v, --version\t crd version name. such as[v1beta1]"
    echo -e "\t-k, --kind\t crd kind name. It must be singular, such as [Sample]"
    echo -e "\t-sn, --shortname\t crd kind short name. such as [s]"
    echo -e "\t-s, --scope\t crd scoped , support [${SCOPE_NAMESPACE} ${SCOPE_CLUSTER}]."
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

echo "Add controller Group: $GROUP Version: $VERSION Instance Kind: $KIND_FIRST_UPPER ShortName: $SHORTNAME"

if [ $SCOPE != $SCOPE_NAMESPACE ] && [ $SCOPE != $SCOPE_CLUSTER ]; then
    echo "scope only support [$SCOPE_NAMESPACE $SCOPE_CLUSTER]"
    exit 1
fi

PKG_DIR=${YURT_ROOT}/pkg
APIS_DIR=${PKG_DIR}/apis
CONTROLLER_DIR=${PKG_DIR}/controller
WEBHOOK_DIR=${PKG_DIR}/webhook

if [ ! -d ${PKG_DIR} ] || [ ! -d ${APIS_DIR} ] || [ ! -d ${CONTROLLER_DIR} ] || [ ! -d ${WEBHOOK_DIR} ] ; then
    echo "Please check pkg、apis、controller、webhook dir ..."
    exit 1
fi


CRD_GROUP_DIR=${APIS_DIR}/${GROUP}
CRD_VERSION_DIR=${CRD_GROUP_DIR}/${VERSION}


CRD_KIND_FILE=${CRD_VERSION_DIR}/${KIND_ALL_LOWER}_types.go
CRD_VERSION_DEFAULT_FILE=${CRD_VERSION_DIR}/default.go 
KIND_CONTROLLER_DIR=${CONTROLLER_DIR}/${KIND_ALL_LOWER}
KIND_CONTROLLER_FILE=${KIND_CONTROLLER_DIR}/${KIND_ALL_LOWER}_controller.go
ADD_CONTROLLER_FILE=${CONTROLLER_DIR}/add_${KIND_ALL_LOWER}.go

WEBHOOK_KIND_DIR=${WEBHOOK_DIR}/${KIND_ALL_LOWER}
ADD_WEBHOOK_FILE=${WEBHOOK_DIR}/add_${KIND_ALL_LOWER}.go
    
WEBHOOK_KIND_MUTATING_DIR=${WEBHOOK_KIND_DIR}/mutating
KIND_MUTATING_HANDLER_FILE=${WEBHOOK_KIND_MUTATING_DIR}/${KIND_ALL_LOWER}_handler.go
KIND_MUTATING_WEBHOOKS_FILE=${WEBHOOK_KIND_MUTATING_DIR}/webhooks.go

WEBHOOK_KIND_VALIDATING_DIR=${WEBHOOK_KIND_DIR}/validating
KIND_VALIDATING_HANDLER_FILE=${WEBHOOK_KIND_VALIDATING_DIR}/${KIND_ALL_LOWER}_handler.go
KIND_VALIDATING_WEBHOOKS_FILE=${WEBHOOK_KIND_VALIDATING_DIR}/webhooks.go

if [ -f "${CRD_KIND_FILE}" ]; then
    echo "Instance crd[${GROUP}/${VERSION}/${KIND_ALL_LOWER}] already exist ..." 
    exit 1
fi

if [ -f "${ADD_CONTROLLER_FILE}" ]; then
    echo "Add controller file ${ADD_CONTROLLER_FILE} already exist ..."
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
    local doc_file=$CRD_VERSION_DIR/doc.go 
    local group_version_info_file=$CRD_VERSION_DIR/groupversion_info.go

    mkdir -p ${CRD_VERSION_DIR}

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

    cat > $CRD_VERSION_DEFAULT_FILE <<EOF
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
        echo "Group ${GROUP} not exist, Do you want to create it?[Y/n]"
        read create_group 
        if [ $create_group="Y" ]; then
            mkdir -p ${CRD_GROUP_DIR}
        else
            exit 0
        fi
    fi

    if [ ! -d ${CRD_VERSION_DIR} ]; then
        echo "Version ${VERSION} not exist, Do you want to create it?[Y/n]"
        read create_version
        if [ $create_version="Y" ]; then
            build_new_version_frame
            need_create_version_dir="True"
        else
            exit 0
        fi
    fi


    local addtoscheme_group_version_file=${APIS_DIR}/addtoscheme_${GROUP}_${VERSION}.go

    if [ $need_create_version_dir="True" ]; then
        create_addtoscheme_group_version_file $addtoscheme_group_version_file
    fi

    cat > ${CRD_KIND_FILE} << EOF
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
    cat >> $CRD_VERSION_DEFAULT_FILE << EOF

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

    # create controller file 
    cat > $KIND_CONTROLLER_FILE << EOF
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

	utilclient "github.com/openyurtio/openyurt/pkg/util/client"
	utildiscovery "github.com/openyurtio/openyurt/pkg/util/discovery"
	${GROUP}${VERSION} "github.com/openyurtio/openyurt/pkg/apis/${GROUP}/${VERSION}"
)

func init() {
	flag.IntVar(&concurrentReconciles, "${KIND_ALL_LOWER}-workers", concurrentReconciles, "Max concurrent workers for $KIND_FIRST_UPPER controller.")
}

var (
	concurrentReconciles = 3
	controllerKind       = ${GROUP}${VERSION}.SchemeGroupVersion.WithKind("${KIND_FIRST_UPPER}")
)

const (
	controllerName = "${KIND_FIRST_UPPER}-controller"
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", controllerName, s)
}

// Add creates a new ${KIND_FIRST_UPPER} Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	if !utildiscovery.DiscoverGVK(controllerKind) {
		return nil
	}
	return add(mgr, newReconciler(mgr))
}

var _ reconcile.Reconciler = &Reconcile${KIND_FIRST_UPPER}{}

// Reconcile${KIND_FIRST_UPPER} reconciles a ${KIND_FIRST_UPPER} object
type Reconcile${KIND_FIRST_UPPER} struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &Reconcile${KIND_FIRST_UPPER}{
		Client:   utilclient.NewClientFromManager(mgr, controllerName),
		scheme:   mgr.GetScheme(),
		recorder: mgr.GetEventRecorderFor(controllerName),
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
	klog.Infof(Format("Reconcile ${KIND_FIRST_UPPER} %s/%s", request.Namespace, request.Name))

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
			klog.Errorf(Format("Update ${KIND_FIRST_UPPER} Status %s error %v", klog.KObj(instance), err))
			return reconcile.Result{Requeue: true}, err
		}
	}

    // Update Instance
	//if err = r.Update(context.TODO(), instance); err != nil {
	//	klog.Errorf(Format("Update ${KIND_FIRST_UPPER} %s error %v", klog.KObj(instance), err))
	//	return reconcile.Result{Requeue: true}, err
	//}


	return reconcile.Result{}, nil
}

EOF


    
    # ADD_CONTROLLER_FILE
    cat > ${ADD_CONTROLLER_FILE} <<EOF
$(create_header controller)

import (
    "github.com/openyurtio/openyurt/pkg/controller/${KIND_ALL_LOWER}"
)

// Note !!! @kadisi
// Do not change the name of the file @kadisi
// Auto generate by make addcontroller command !!!
// Note !!!

func init() {
    controllerAddFuncs = append(controllerAddFuncs, ${KIND_ALL_LOWER}.Add)
}

EOF

    gofmt -w ${ADD_CONTROLLER_FILE}
    goimports -w ${ADD_CONTROLLER_FILE}
    
}


function build_webhook_frame() {

    if [ -f ${ADD_WEBHOOK_FILE} ]; then
        echo "${ADD_WEBHOOK_FILE} file has exist ..."
        exit 1
    else
        cat > ${ADD_WEBHOOK_FILE} <<EOF
$(create_header webhook)

import (
	"github.com/openyurtio/openyurt/pkg/webhook/${KIND_ALL_LOWER}/mutating"
	"github.com/openyurtio/openyurt/pkg/webhook/${KIND_ALL_LOWER}/validating"
)

func init() {
	addHandlers(mutating.HandlerMap)
	addHandlers(validating.HandlerMap)
}

EOF

    fi

    if [ -d ${WEBHOOK_KIND_MUTATING_DIR} ]; then
        echo "${WEBHOOK_KIND_MUTATING_DIR} dir has exist ..."
        exit 1
    fi 

    if [ -d ${WEBHOOK_KIND_VALIDATING_DIR} ]; then
       echo "${WEBHOOK_KIND_VALIDATING_DIR} dir has exist ..."
       exit 1
    fi
    
    mkdir -p ${WEBHOOK_KIND_MUTATING_DIR}
    mkdir -p ${WEBHOOK_KIND_VALIDATING_DIR}
    
   cat > $KIND_MUTATING_HANDLER_FILE << EOF
$(create_header mutating)

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/openyurtio/openyurt/pkg/util"
	${GROUP}${VERSION} "github.com/openyurtio/openyurt/pkg/apis/${GROUP}/${VERSION}"
)

const (
	webhookName = "${KIND_FIRST_UPPER}-mutate-webhook"
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", webhookName, s)
}

// ${KIND_FIRST_UPPER}CreateUpdateHandler handles ${KIND_FIRST_UPPER} 
type ${KIND_FIRST_UPPER}CreateUpdateHandler struct {
	// Decoder decodes objects
	Decoder *admission.Decoder
}

var _ admission.Handler = &${KIND_FIRST_UPPER}CreateUpdateHandler{}

// Handle handles admission requests.
func (h *${KIND_FIRST_UPPER}CreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {

	// Note !!!!!!!!!!
	// We strongly recommend use Format() to  encapsulation because Format() can print logs by module
	// @kadisi
	klog.Infof(Format("Handle ${KIND_FIRST_UPPER} %s/%s", req.Namespace, req.Name))

	obj := &${GROUP}${VERSION}.${KIND_FIRST_UPPER}{}
	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	var copy runtime.Object = obj.DeepCopy()
	// Set defaults
	${GROUP}${VERSION}.SetDefaults${KIND_FIRST_UPPER}(obj)

	if reflect.DeepEqual(obj, copy) {
		return admission.Allowed("")
	}
	marshalled, err := json.Marshal(obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	resp := admission.PatchResponseFromRaw(req.AdmissionRequest.Object.Raw, marshalled)
	if len(resp.Patches) > 0 {
		klog.Infof(Format("Admit ${KIND_FIRST_UPPER} %s patches: %v", obj.Name, util.DumpJSON(resp.Patches)))
	}

	return resp
}

var _ admission.DecoderInjector = &${KIND_FIRST_UPPER}CreateUpdateHandler{}

// InjectDecoder injects the decoder into the ${KIND_FIRST_UPPER}CreateUpdateHandler
func (h *${KIND_FIRST_UPPER}CreateUpdateHandler) InjectDecoder(d *admission.Decoder) error {
	h.Decoder = d
	return nil
}

EOF

    gofmt -w ${KIND_MUTATING_HANDLER_FILE}
    goimports -w ${KIND_MUTATING_HANDLER_FILE}

    cat > $KIND_MUTATING_WEBHOOKS_FILE << EOF
$(create_header mutating)

import (
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/mutate-${GROUP}-openyurt-io-${VERSION}-${KIND_ALL_LOWER},mutating=true,failurePolicy=fail,sideEffects=None,admissionReviewVersions=v1;v1beta1,groups=${GROUP}.openyurt.io,resources=${KIND_PLURAL},verbs=create;update,versions=${VERSION},name=mutate.${GROUP}.${VERSION}.${KIND_ALL_LOWER}.openyurt.io

var (
	// HandlerMap contains admission webhook handlers
	HandlerMap = map[string]admission.Handler{
		"mutate-${GROUP}-openyurt-io-${VERSION}-${KIND_ALL_LOWER}": &${KIND_FIRST_UPPER}CreateUpdateHandler{},
	}
)
EOF

    cat > $KIND_VALIDATING_HANDLER_FILE << EOF
$(create_header validating)

import (
	"context"
	"fmt"
	"net/http"

	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	${GROUP}${VERSION} "github.com/openyurtio/openyurt/pkg/apis/${GROUP}/${VERSION}"
)

// ${KIND_FIRST_UPPER}CreateUpdateHandler handles ${KIND_FIRST_UPPER}
type ${KIND_FIRST_UPPER}CreateUpdateHandler struct {
	// Decoder decodes objects
	Decoder *admission.Decoder
}

const (
	webhookName = "${KIND_FIRST_UPPER}-validate-webhook"
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", webhookName, s)
}

var _ admission.Handler = &${KIND_FIRST_UPPER}CreateUpdateHandler{}

// Handle handles admission requests.
func (h *${KIND_FIRST_UPPER}CreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	// Note !!!!!!!!!!
	// We strongly recommend use Format() to  encapsulation because Format() can print logs by module
	// @kadisi
	klog.Infof(Format("Handle ${KIND_FIRST_UPPER} %s/%s", req.Namespace, req.Name))

	obj := &${GROUP}${VERSION}.${KIND_FIRST_UPPER}{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if err := validate(obj); err != nil {
		klog.Warningf("Error validate ${KIND_FIRST_UPPER} %s: %v", obj.Name, err)
		return admission.Errored(http.StatusBadRequest, err)
	}

	return admission.ValidationResponse(true, "allowed")
}

func validate(obj *${GROUP}${VERSION}.${KIND_FIRST_UPPER}) error {

	klog.Infof(Format("Validate ${KIND_FIRST_UPPER} %s successfully ...", klog.KObj(obj)))

	return nil
}

var _ admission.DecoderInjector = &${KIND_FIRST_UPPER}CreateUpdateHandler{}

// InjectDecoder injects the decoder into the ${KIND_FIRST_UPPER}CreateUpdateHandler
func (h *${KIND_FIRST_UPPER}CreateUpdateHandler) InjectDecoder(d *admission.Decoder) error {
	h.Decoder = d
	return nil
}

EOF

    gofmt -w $KIND_VALIDATING_HANDLER_FILE
    goimports -w $KIND_VALIDATING_HANDLER_FILE

    cat > $KIND_VALIDATING_WEBHOOKS_FILE << EOF
$(create_header validating)

import (
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/validate-${GROUP}-openyurt-io-${VERSION}-${KIND_ALL_LOWER},mutating=false,failurePolicy=fail,sideEffects=None,admissionReviewVersions=v1;v1beta1,groups=${GROUP}.openyurt.io,resources=${KIND_PLURAL},verbs=create;update,versions=${VERSION},name=validate.${GROUP}.${VERSION}.${KIND_ALL_LOWER}.openyurt.io

var (
	// HandlerMap contains admission webhook handlers
	HandlerMap = map[string]admission.Handler{
		"validate-${GROUP}-openyurt-io-${VERSION}-${KIND_ALL_LOWER}": &${KIND_FIRST_UPPER}CreateUpdateHandler{},
	}
)

EOF

}


build_apis_frame
build_controller_frame
build_webhook_frame
