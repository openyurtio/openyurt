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
    echo -e "\t-i, --instance\t crd name. It must be singular, such as [Sample]"
    echo -e "\t-sn, --shortname\t crd instance short name. such as [s]"
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
    --instance|-i)
      shift
      INSTANCE=$1
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

if [ -z $GROUP ] || [ -z $VERSION ] || [ -z $INSTANCE ] || [ -z $SCOPE ] || [ -z $SHORTNAME ] ; then
    usage	
fi

# suport bash 3 [mac and linux]
# @kadisi

GROUP=$(echo $GROUP | tr '[A-Z]' '[a-z]')
VERSION=$(echo $VERSION | tr '[A-Z]' '[a-z]')
INSTANCE=$(echo $INSTANCE | tr '[A-Z]' '[a-z]')
SHORTNAME=$(echo $SHORTNAME | tr '[A-Z]' '[a-z]')

INSTANCE_PLURAL="${INSTANCE}s"

# suport bash 3 [mac and linux]
# @kadisi

INSTANCE_INITIAL=$(echo ${INSTANCE: 0:1} | tr '[a-z]' '[A-Z]')
INSTANCE_FIRST_UPPER=${INSTANCE_INITIAL}${INSTANCE: 1}

echo "Add controller Group: $GROUP, Version: $VERSION, Instance: $INSTANCE ,ShortName: $SHORTNAME"

if [ $SCOPE != $SCOPE_NAMESPACE ] && [ $SCOPE != $SCOPE_CLUSTER ]; then
    echo "scope only support [$SCOPE_NAMESPACE $SCOPE_CLUSTER]"
fi

PKG_DIR=${YURT_ROOT}/pkg
APIS_DIR=${PKG_DIR}/apis
CONTROLLER_DIR=${PKG_DIR}/controller
WEBHOOK_DIR=${PKG_DIR}/webhook

if [ ! -d ${PKG_DIR} ] || [ ! -d ${APIS_DIR} ] || [ ! -d ${CONTROLLER_DIR} ] || [ ! -d ${WEBHOOK_DIR} ] ; then
    echo "Please check pkg、apis、controller、webhook dir ..."
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
    local crd_group_dir=$1
    local crd_version_dir=$2
    local default_file=$3
    local group=$4
    local version=$5

    local doc_file=$crd_version_dir/doc.go 
    local group_version_info_file=$crd_version_dir/groupversion_info.go

    mkdir -p ${crd_version_dir}

    cat > $doc_file << EOF
$(create_header ${version})
EOF

    cat > $group_version_info_file << EOF
$(create_header ${version})

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	// GroupVersion is group version used to register these objects
	GroupVersion = schema.GroupVersion{Group: "${group}.openyurt.io", Version: "${version}"}

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

    cat > $default_file << EOF
$(create_header ${version})

EOF
   
}

function build_apis_frame() {

    local crd_group_dir=${APIS_DIR}/${GROUP}
    local crd_version_dir=${crd_group_dir}/${VERSION}
    local crd_instance_file=${crd_version_dir}/${INSTANCE}_types.go
    local default_file=${crd_version_dir}/default.go 
    local need_create_version_dir="False"

    if [ -f ${crd_instance_file} ]; then
        echo "Instance crd[${GROUP}/${VERSION}/${INSTANCE}] already exist ..." 
        exit 1
    if

    if [ ! -d ${crd_group_dir} ]; then
        echo "Group ${GROUP} not exist, Do you want to create it?[Y/n]"
        read create_group 
        if [ $create_group == "Y" ]; then
            mkdir -p ${crd_group_dir}
        else
            exit 0
        fi
    fi


    if [ ! -d ${crd_version_dir} ]; then
        echo "Version ${VERSION} not exist, Do you want to create it?[Y/n]"
        read create_version
        if [ $create_version == "Y" ]; then
            build_new_version_frame ${crd_group_dir} ${crd_version_dir} ${default_file} ${GROUP} ${VERSION}
            need_create_version_dir="True"
        else
            exit 0
        fi
    fi


    local addtoscheme_group_version_file=${APIS_DIR}/addtoscheme_${GROUP}_${VERSION}.go

    if [ $need_create_version_dir = "True" ]; then
        cat > $addtoscheme_group_version_file << EOF
$(create_header apis)
import (
	version "github.com/openyurtio/openyurt/pkg/apis/${GROUP}/${VERSION}"
)

func init() {
	// Register the types with the Scheme so the components can map objects to GroupVersionKinds and back
	AddToSchemes = append(AddToSchemes, version.SchemeBuilder.AddToScheme)
}

EOF 

    fi

    cat > ${crd_instance_file} << EOF

$(create_header ${VERSION})

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ${INSTANCE_FIRST_UPPER}Spec defines the desired state of ${INSTANCE_FIRST_UPPER} 
type ${INSTANCE_FIRST_UPPER}Spec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of ${INSTANCE_FIRST_UPPER}. Edit sample_types.go to remove/update
	Foo string `json:"foo,omitempty"`

	// Default is an example field of ${INSTANCE_FIRST_UPPER}. Edit sample_types.go to remove/update
	Default string `json:"default,omitempty"`
}

// ${INSTANCE_FIRST_UPPER}Status defines the observed state of ${INSTANCE_FIRST_UPPER} 
type ${INSTANCE_FIRST_UPPER}Status struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of ${INSTANCE_FIRST_UPPER}. Edit sample_types.go to remove/update
	Foo string `json:"foo,omitempty"`

	// Default is an example field of ${INSTANCE_FIRST_UPPER}. Edit sample_types.go to remove/update
	Default string `json:"default,omitempty"`
}

// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=${SCOPE},path=${INSTANCE_PLURAL},shortName=${SHORTNAME},categories=all
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp",description="CreationTimestamp is a timestamp representing the server time when this object was created. It is not guaranteed to be set in happens-before order across separate operations. Clients may not set this value. It is represented in RFC3339 form and is in UTC."

// ${INSTANCE_FIRST_UPPER} is the Schema for the samples API
type ${INSTANCE_FIRST_UPPER} struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ${INSTANCE_FIRST_UPPER}Spec   `json:"spec,omitempty"`
	Status ${INSTANCE_FIRST_UPPER}Status `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ${INSTANCE_FIRST_UPPER}List contains a list of ${INSTANCE_FIRST_UPPER} 
type ${INSTANCE_FIRST_UPPER}List struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []${INSTANCE_FIRST_UPPER} `json:"items"`
}

func init() {
	SchemeBuilder.Register(&${INSTANCE_FIRST_UPPER}{}, &${INSTANCE_FIRST_UPPER}List{})
}

EOF

    # set version_default file
    cat >> $default_file << EOF
// SetDefaults${INSTANCE_FIRST_UPPER} set default values for ${INSTANCE_FIRST_UPPER}.
func SetDefaults${INSTANCE_FIRST_UPPER}(obj *${INSTANCE_FIRST_UPPER}) {
	// example for set default value for ${INSTANCE_FIRST_UPPER} 
}

EOF

}


function build_controller_frame() {
    local global_controller_file=${CONTROLLER_DIR}/controller.go
    local instance_controller_dir=${CONTROLLER_DIR}/${INSTANCE}
    local instance_controller_file=${instance_controller_dir}/${INSTANCE}_controller.go

    # create instance controller 
    mkdir -p ${instance_controller_dir}

    # create controller file 
    cat > <<EOF
$(create_header ${INSTANCE})


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
	flag.IntVar(&concurrentReconciles, "${INSTANCE}-workers", concurrentReconciles, "Max concurrent workers for $INSTANCE_FIRST_UPPER controller.")
}

var (
	concurrentReconciles = 3
	controllerKind       = ${GROUP}${VERSION}.SchemeGroupVersion.WithKind("${INSTANCE_FIRST_UPPER}")
)

const (
	controllerName = "${INSTANCE_FIRST_UPPER}-controller"
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", controllerName, s)
}

// Add creates a new ${INSTANCE_FIRST_UPPER} Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	if !utildiscovery.DiscoverGVK(controllerKind) {
		return nil
	}
	return add(mgr, newReconciler(mgr))
}

var _ reconcile.Reconciler = &Reconcile${INSTANCE_FIRST_UPPER}{}

// Reconcile${INSTANCE_FIRST_UPPER} reconciles a ${INSTANCE_FIRST_UPPER} object
type Reconcile${INSTANCE_FIRST_UPPER} struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileSample{
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

	// Watch for changes to ${INSTANCE_FIRST_UPPER} 
	err = c.Watch(&source.Kind{Type: &${GROUP}${VERSION}.${INSTANCE_FIRST_UPPER}{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

// +kubebuilder:rbac:groups=${GROUP}.openyurt.io,resources=${INSTANCE_PLURAL},verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=${GROUP}.openyurt.io,resources=${INSTANCE_PLURAL}/status,verbs=get;update;patch

// Reconcile reads that state of the cluster for a ${INSTANCE_FIRST_UPPER} object and makes changes based on the state read
// and what is in the ${INSTANCE_FIRST_UPPER}.Spec
func (r *Reconcile${INSTANCE_FIRST_UPPER}) Reconcile(_ context.Context, request reconcile.Request) (reconcile.Result, error) {

	// Note !!!!!!!!!!
	// We strongly recommend use Format() to  encapsulation because Format() can print logs by module
	// @kadisi
	klog.Infof(Format("Reconcile ${INSTANCE_FIRST_UPPER} %s/%s", request.Namespace, request.Name))

	// Fetch the ${INSTANCE_FIRST_UPPER} instance
	instance := &${GROUP}${VERSION}.${INSTANCE_FIRST_UPPER}{}
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
/*
	if instance.Spec.Foo != instance.Status.Foo {
		instance.Status.Foo = instance.Spec.Foo
		if err = r.Status().Update(context.TODO(), instance); err != nil {
			klog.Errorf(Format("Update ${INSTANCE_FIRST_UPPER} Status %s error %v", klog.KObj(instance), err))
			return reconcile.Result{Requeue: true}, err
		}
	}
	if err = r.Update(context.TODO(), instance); err != nil {
		klog.Errorf(Format("Update ${INSTANCE_FIRST_UPPER} %s error %v", klog.KObj(instance), err))
		return reconcile.Result{Requeue: true}, err
	}
*/

	return reconcile.Result{}, nil
}

EOF


    # update global controller file
    if [ "$(uname)"=="Darwin" ];then
        # Mac OS X 
        sed -i '' '/import (/a\'$'\n    "github.com/openyurtio/openyurt/pkg/controller/'"${INSTANCE}"'"'$'\n' ${global_controller_file}
        sed -i '' '/func init() {/a\'$'\n    controllerAddFuncs = append(controllerAddFuncs, '"${INSTANCE}"'.Add)'$'\n' ${global_controller_file} 
    elif[ "$(expr substr $(uname -s) 1 5)"=="Linux" ];then   
        # GNU/Linux
        sed -i '/import (/a"github.com/openyurtio/openyurt/pkg/controller/'"${INSTANCE}"'"' ${global_controller_file}
        sed -i '/func init() {/a controllerAddFuncs = append(controllerAddFuncs, '"${INSTANCE}"'.Add)' ${global_controller_file}
    fi    
    go fmt ${global_controller_file}
    go import ${global_controller_file}
    

}

function build_webhook_frame() {
    local add_webhook_file=${WEBHOOK_DIR}/add_${INSTANCE}.go

    local webhook_instance_dir=${WEBHOOK_DIR}/${INSTANCE}
    
    local mutating_dir=${webhook_instance_dir}/mutating
    local mutating_handler_file=${mutating_dir}/${INSTANCE}_handler.go
    local mutating_webhooks_file=${mutating_dir}/webhooks.go

    local validating_dir=${webhook_instance_dir}/validating
    local validating_handler_file=${validating_dir}/${INSTANCE}_handler.go
    local validating_webhooks_file=${validating_dir}/webhooks.go

    if [ -f ${add_webhook_file} ]; then
        echo "${add_webhook_file} file has exist ..."
        exit 1
    fi

    if [ -d ${mutating_dir} ]; then
        echo "${mutating_dir} dir has exist ..."
        exit 1
    fi 

    if [ -d ${validating_dir} ]; then
       echo "${validating_dir} dir has exist ..."
       exit 1
    fi
    
    mkdir -p ${mutating_dir}
    mkdir -p ${validating_dir}
    
   cat > $mutating_handler_file << EOF
$(create_header mutating)

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"

	${GROUP}${VERSION} "github.com/openyurtio/openyurt/pkg/apis/${GROUP}/${VERSION}"
	"github.com/openyurtio/openyurt/pkg/util"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	webhookName = "${INSTANCE_FIRST_UPPER}-mutate-webhook"
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", webhookName, s)
}

// ${INSTANCE_FIRST_UPPER}CreateUpdateHandler handles ${INSTANCE_FIRST_UPPER} 
type ${INSTANCE_FIRST_UPPER}CreateUpdateHandler struct {
	// Decoder decodes objects
	Decoder *admission.Decoder
}

var _ admission.Handler = &${INSTANCE_FIRST_UPPER}CreateUpdateHandler{}

// Handle handles admission requests.
func (h *${INSTANCE_FIRST_UPPER}CreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {

	// Note !!!!!!!!!!
	// We strongly recommend use Format() to  encapsulation because Format() can print logs by module
	// @kadisi
	klog.Infof(Format("Handle ${INSTANCE_FIRST_UPPER} %s/%s", req.Namespace, req.Name))

	obj := &${GROUP}${VERSION}.${INSTANCE_FIRST_UPPER}{}
	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	var copy runtime.Object = obj.DeepCopy()
	// Set defaults
	${GROUP}${VERSION}.SetDefaults${INSTANCE_FIRST_UPPER}(obj)

	if reflect.DeepEqual(obj, copy) {
		return admission.Allowed("")
	}
	marshalled, err := json.Marshal(obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	resp := admission.PatchResponseFromRaw(req.AdmissionRequest.Object.Raw, marshalled)
	if len(resp.Patches) > 0 {
		klog.Infof(Format("Admit ${INSTANCE_FIRST_UPPER} %s patches: %v", obj.Name, util.DumpJSON(resp.Patches)))
	}

	return resp
}

var _ admission.DecoderInjector = &${INSTANCE_FIRST_UPPER}CreateUpdateHandler{}

// InjectDecoder injects the decoder into the ${INSTANCE_FIRST_UPPER}CreateUpdateHandler
func (h *${INSTANCE_FIRST_UPPER}CreateUpdateHandler) InjectDecoder(d *admission.Decoder) error {
	h.Decoder = d
	return nil
}

EOF 

    cat > $mutating_webhooks_file << EOF
$(create_header mutating)

import (
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/mutate-${GROUP}-openyurt-io-${VERSION}-${INSTANCE},mutating=true,failurePolicy=fail,sideEffects=None,admissionReviewVersions=v1;v1beta1,groups=${GROUP}.openyurt.io,resources=${INSTANCE_PLURAL},verbs=create;update,versions=${VERSION},name=mutate.${GROUP}.${VERSION}.${INSTANCE}.openyurt.io

var (
	// HandlerMap contains admission webhook handlers
	HandlerMap = map[string]admission.Handler{
		"mutate-${GROUP}-openyurt-io-${VERSION}-${INSTANCE}": &${INSTANCE_FIRST_UPPER}CreateUpdateHandler{},
	}
)
EOF

    cat > $validating_handler_file << EOF
$(create_header validating)

import (
	"context"
	"fmt"
	"net/http"

	${GROUP}${VERSION} "github.com/openyurtio/openyurt/pkg/apis/${GROUP}/${VERSION}"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	defaultMaxImagesPerNode = 256
)

// ${INSTANCE_FIRST_UPPER}CreateUpdateHandler handles ${INSTANCE_FIRST_UPPER} 
type ${INSTANCE_FIRST_UPPER}CreateUpdateHandler struct {
	// Decoder decodes objects
	Decoder *admission.Decoder
}

const (
	webhookName = "${INSTANCE_FIRST_UPPER}-validate-webhook"
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", webhookName, s)
}

var _ admission.Handler = &${INSTANCE_FIRST_UPPER}CreateUpdateHandler{}

// Handle handles admission requests.
func (h *${INSTANCE_FIRST_UPPER}CreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	// Note !!!!!!!!!!
	// We strongly recommend use Format() to  encapsulation because Format() can print logs by module
	// @kadisi
	klog.Infof(Format("Handle ${INSTANCE_FIRST_UPPER} %s/%s", req.Namespace, req.Name))

	obj := &${GROUP}${VERSION}.${INSTANCE_FIRST_UPPER}{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if err := validate(obj); err != nil {
		klog.Warningf("Error validate ${INSTANCE_FIRST_UPPER} %s: %v", obj.Name, err)
		return admission.Errored(http.StatusBadRequest, err)
	}

	return admission.ValidationResponse(true, "allowed")
}

func validate(obj *${GROUP}${VERSION}.${INSTANCE_FIRST_UPPER}) error {

	klog.Infof(Format("Validate ${INSTANCE_FIRST_UPPER} %s sucessfully ...", klog.KObj(obj)))

	return nil
}

var _ admission.DecoderInjector = &${INSTANCE_FIRST_UPPER}CreateUpdateHandler{}

// InjectDecoder injects the decoder into the ${INSTANCE_FIRST_UPPER}CreateUpdateHandler
func (h *${INSTANCE_FIRST_UPPER}CreateUpdateHandler) InjectDecoder(d *admission.Decoder) error {
	h.Decoder = d
	return nil
}

EOF

    cat > $validating_webhooks_file << EOF
$(create_header validating)

import (
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/validate-${GROUP}-openyurt-io-${VERSION}-${INSTANCE},mutating=false,failurePolicy=fail,sideEffects=None,admissionReviewVersions=v1;v1beta1,groups=${GROUP}.openyurt.io,resources=${INSTANCE_PLURA},verbs=create;update,versions=${VERSION},name=validate.${GROUP}.${VERSION}.${INSTANCE}.openyurt.io

var (
	// HandlerMap contains admission webhook handlers
	HandlerMap = map[string]admission.Handler{
		"validate-${GROUP}-openyurt-io-${VERSION}-${INSTANCE}": &${INSTANCE_FIRST_UPPER}CreateUpdateHandler{},
	}
)

EOF


}

build_apis_frame
build_controller_frame
build_webhook_frame

