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
package v3

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/edgexfoundry/go-mod-core-contracts/v3/dtos"
	edgex_resp "github.com/edgexfoundry/go-mod-core-contracts/v3/dtos/responses"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"

	"github.com/openyurtio/openyurt/pkg/yurtiotdock/clients"
)

const (
	DeviceServiceListMetaData = `{"apiVersion":"v3","statusCode":200,"totalCount":1,"services":[{"created":1661829206490,"modified":1661850999190,"id":"74516e96-973d-4cad-bad1-afd4b3a8ea46","name":"device-virtual","baseAddress":"http://edgex-device-virtual:59900","adminState":"UNLOCKED"}]}`
	DeviceServiceMetaData     = `{"apiVersion":"v3","statusCode":200,"service":{"created":1661829206490,"modified":1661850999190,"id":"74516e96-973d-4cad-bad1-afd4b3a8ea46","name":"device-virtual","baseAddress":"http://edgex-device-virtual:59900","adminState":"UNLOCKED"}}`
	ServiceCreateSuccess      = `[{"apiVersion":"v3","statusCode":201,"id":"a583b97d-7c4d-4b7c-8b93-51da9e68518c"}]`
	ServiceCreateFail         = `[{"apiVersion":"v3","message":"device service name test-device-virtual exists","statusCode":409}]`

	ServiceDeleteSuccess = `{"apiVersion":"v3","statusCode":200}`
	ServiceDeleteFail    = `{"apiVersion":"v3","message":"fail to delete the device profile with name test-Random-Boolean-Device","statusCode":404}`

	ServiceUpdateSuccess = `[{"apiVersion":"v3","statusCode":200}]`
	ServiceUpdateFail    = `[{"apiVersion":"v3","message":"fail to query object *models.DeviceService, because id: md|ds:01dfe04d-f361-41fd-b1c4-7ca0718f461a doesn't exist in the database","statusCode":404}]`

	ServiceSystemEvent = `{"apiVersion":"v3","type":"deviceservice","action":"add","source":"core-metadata","owner":"device-onvif-camera","tags":{},"details":{"modified":1661850999190,"id":"74516e96-973d-4cad-bad1-afd4b3a8ea46","name":"device-virtual","baseAddress":"http://edgex-device-virtual:59900","adminState":"UNLOCKED"},"timestamp":1639279435789056000}`
)

var serviceClient = NewEdgexDeviceServiceClient("edgex-core-metadata:59881")

func Test_GetService(t *testing.T) {
	httpmock.ActivateNonDefault(serviceClient.Client.GetClient())
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("GET", "http://edgex-core-metadata:59881/api/v3/deviceservice/name/device-virtual",
		httpmock.NewStringResponder(200, DeviceServiceMetaData))

	_, err := serviceClient.Get(context.TODO(), "device-virtual", clients.GetOptions{Namespace: "default"})
	assert.Nil(t, err)
}

func Test_ListService(t *testing.T) {
	httpmock.ActivateNonDefault(serviceClient.Client.GetClient())
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("GET", "http://edgex-core-metadata:59881/api/v3/deviceservice/all?limit=-1",
		httpmock.NewStringResponder(200, DeviceServiceListMetaData))

	services, err := serviceClient.List(context.TODO(), clients.ListOptions{})
	assert.Nil(t, err)
	assert.Equal(t, 1, len(services))
}

func Test_CreateService(t *testing.T) {
	httpmock.ActivateNonDefault(serviceClient.Client.GetClient())
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("POST", "http://edgex-core-metadata:59881/api/v3/deviceservice",
		httpmock.NewStringResponder(207, ServiceCreateSuccess))

	var resp edgex_resp.DeviceServiceResponse

	err := json.Unmarshal([]byte(DeviceServiceMetaData), &resp)
	assert.Nil(t, err)

	service := toKubeDeviceService(resp.Service, "default")
	service.Name = "test-device-virtual"

	_, err = serviceClient.Create(context.TODO(), &service, clients.CreateOptions{})
	assert.Nil(t, err)

	httpmock.RegisterResponder("POST", "http://edgex-core-metadata:59881/api/v3/deviceservice",
		httpmock.NewStringResponder(207, ServiceCreateFail))
}

func Test_DeleteService(t *testing.T) {
	httpmock.ActivateNonDefault(serviceClient.Client.GetClient())
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("DELETE", "http://edgex-core-metadata:59881/api/v3/deviceservice/name/test-device-virtual",
		httpmock.NewStringResponder(200, ServiceDeleteSuccess))

	err := serviceClient.Delete(context.TODO(), "test-device-virtual", clients.DeleteOptions{})
	assert.Nil(t, err)

	httpmock.RegisterResponder("DELETE", "http://edgex-core-metadata:59881/api/v3/deviceservice/name/test-device-virtual",
		httpmock.NewStringResponder(404, ServiceDeleteFail))

	err = serviceClient.Delete(context.TODO(), "test-device-virtual", clients.DeleteOptions{})
	assert.NotNil(t, err)
}

func Test_UpdateService(t *testing.T) {
	httpmock.ActivateNonDefault(serviceClient.Client.GetClient())
	defer httpmock.DeactivateAndReset()
	httpmock.RegisterResponder("PATCH", "http://edgex-core-metadata:59881/api/v3/deviceservice",
		httpmock.NewStringResponder(200, ServiceUpdateSuccess))
	var resp edgex_resp.DeviceServiceResponse

	err := json.Unmarshal([]byte(DeviceServiceMetaData), &resp)
	assert.Nil(t, err)

	service := toKubeDeviceService(resp.Service, "default")
	_, err = serviceClient.Update(context.TODO(), &service, clients.UpdateOptions{})
	assert.Nil(t, err)

	httpmock.RegisterResponder("PATCH", "http://edgex-core-metadata:59881/api/v3/deviceservice",
		httpmock.NewStringResponder(404, ServiceUpdateFail))

	_, err = serviceClient.Update(context.TODO(), &service, clients.UpdateOptions{})
	assert.NotNil(t, err)
}

func Test_ConvertServiceSystemEvents(t *testing.T) {
	dsse := dtos.SystemEvent{}
	err := json.Unmarshal([]byte(ServiceSystemEvent), &dsse)
	if err != nil {
		return
	}

	service, err := serviceClient.Convert(context.TODO(), dsse, clients.GetOptions{Namespace: "default"})
	assert.Nil(t, err)
	fmt.Println(service)
	assert.Equal(t, "device-virtual", service.Name)
	assert.Equal(t, "http://edgex-device-virtual:59900", service.Spec.BaseAddress)
}
