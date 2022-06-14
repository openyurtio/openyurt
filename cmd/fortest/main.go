package main

import (
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/client-go/kubernetes/scheme"
)

var jsonData = `{"kind":"Lease","apiVersion":"coordination.k8s.io/v1","metadata":{"name":"openyurt-e2e-test-worker","namespace":"kube-node-lease","uid":"25f90559-66e8-4d2c-bd8f-de55fa647905","resourceVersion":"1944172","creationTimestamp":"2022-06-06T05:10:22Z","ownerReferences":[{"apiVersion":"v1","kind":"Node","name":"openyurt-e2e-test-worker","uid":"b6db25b2-15d9-43c8-9e87-4f2deb0f298a"}],"managedFields":[{"manager":"kubelet","operation":"Update","apiVersion":"coordination.k8s.io/v1","time":"2022-06-06T05:12:37Z","fieldsType":"FieldsV1","fieldsV1":{"f:metadata":{"f:ownerReferences":{".":{},"k:{\"uid\":\"b6db25b2-15d9-43c8-9e87-4f2deb0f298a\"}":{}}},"f:spec":{"f:holderIdentity":{},"f:leaseDurationSeconds":{}}}},{"manager":"yurthub","operation":"Update","apiVersion":"coordination.k8s.io/v1","time":"2022-06-06T05:12:43Z","fieldsType":"FieldsV1","fieldsV1":{"f:spec":{"f:renewTime":{}}}}]},"spec":{"holderIdentity":"openyurt-e2e-test-worker","leaseDurationSeconds":40,"renewTime":"2022-06-14T08:10:33.309487Z"}}`

func main() {
	gvk, err := json.DefaultMetaFactory.Interpret([]byte(jsonData))
	if err != nil {
		fmt.Printf("interpret err: %v", err)
		return
	}
	var unstructuredObj runtime.Object
	if scheme.Scheme.Recognizes(*gvk) {
		// unstructuredObj = nil
		unstructuredObj = new(unstructured.Unstructured)
	} else {
		unstructuredObj = new(unstructured.Unstructured)
	}
	serializer := json.NewSerializerWithOptions(json.DefaultMetaFactory, scheme.Scheme, scheme.Scheme, json.SerializerOptions{})
	obj, gvk, err := serializer.Decode([]byte(jsonData), nil, unstructuredObj)
	if err != nil {
		fmt.Printf("get err %s\n", err)
		return
	}
	fmt.Printf("gvk: %v\n", gvk)
	fmt.Printf("obj type: %v\nobj: %v\n", reflect.TypeOf(obj), obj)
}
