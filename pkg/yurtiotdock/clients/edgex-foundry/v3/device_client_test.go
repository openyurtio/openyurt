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
	"testing"

	"github.com/edgexfoundry/go-mod-core-contracts/v3/dtos"
	edgex_resp "github.com/edgexfoundry/go-mod-core-contracts/v3/dtos/responses"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"

	iotv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/iot/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/yurtiotdock/clients"
)

const (
	DeviceListMetadata = `{"apiVersion":"v3","statusCode":200,"totalCount":5,"devices":[{"created":1661829206505,"modified":1661829206505,"id":"f6255845-f4b2-4182-bd3c-abc9eac4a649","name":"Random-Float-Device","description":"Example of Device Virtual","adminState":"UNLOCKED","operatingState":"UP","labels":["device-virtual-example"],"serviceName":"device-virtual","profileName":"Random-Float-Device","autoEvents":[{"interval":"30s","onChange":false,"sourceName":"Float32"},{"interval":"30s","onChange":false,"sourceName":"Float64"}],"protocols":{"other":{"Address":"device-virtual-float-01","Protocol":"300"}}},{"created":1661829206506,"modified":1661829206506,"id":"d29efe20-fdec-4aeb-90e5-99528cb6ca28","name":"Random-Binary-Device","description":"Example of Device Virtual","adminState":"UNLOCKED","operatingState":"UP","labels":["device-virtual-example"],"serviceName":"device-virtual","profileName":"Random-Binary-Device","protocols":{"other":{"Address":"device-virtual-binary-01","Port":"300"}}},{"created":1661829206504,"modified":1661829206504,"id":"6a7f00a4-9536-48b2-9380-a9fc202ac517","name":"Random-Integer-Device","description":"Example of Device Virtual","adminState":"UNLOCKED","operatingState":"UP","labels":["device-virtual-example"],"serviceName":"device-virtual","profileName":"Random-Integer-Device","autoEvents":[{"interval":"15s","onChange":false,"sourceName":"Int8"},{"interval":"15s","onChange":false,"sourceName":"Int16"},{"interval":"15s","onChange":false,"sourceName":"Int32"},{"interval":"15s","onChange":false,"sourceName":"Int64"}],"protocols":{"other":{"Address":"device-virtual-int-01","Protocol":"300"}}},{"created":1661829206503,"modified":1661829206503,"id":"439d47a2-fa72-4c27-9f47-c19356cc0c3b","name":"Random-Boolean-Device","description":"Example of Device Virtual","adminState":"UNLOCKED","operatingState":"UP","labels":["device-virtual-example"],"serviceName":"device-virtual","profileName":"Random-Boolean-Device","autoEvents":[{"interval":"10s","onChange":false,"sourceName":"Bool"}],"protocols":{"other":{"Address":"device-virtual-bool-01","Port":"300"}}},{"created":1661829206505,"modified":1661829206505,"id":"2890ab86-3ae4-4b5e-98ab-aad85fc540e6","name":"Random-UnsignedInteger-Device","description":"Example of Device Virtual","adminState":"UNLOCKED","operatingState":"UP","labels":["device-virtual-example"],"serviceName":"device-virtual","profileName":"Random-UnsignedInteger-Device","autoEvents":[{"interval":"20s","onChange":false,"sourceName":"Uint8"},{"interval":"20s","onChange":false,"sourceName":"Uint16"},{"interval":"20s","onChange":false,"sourceName":"Uint32"},{"interval":"20s","onChange":false,"sourceName":"Uint64"}],"protocols":{"other":{"Address":"device-virtual-uint-01","Protocol":"300"}}}]}`
	DeviceMetadata     = `{"apiVersion":"v3","statusCode":200,"device":{"created":1661829206505,"modified":1661829206505,"id":"f6255845-f4b2-4182-bd3c-abc9eac4a649","name":"Random-Float-Device","description":"Example of Device Virtual","adminState":"UNLOCKED","operatingState":"UP","labels":["device-virtual-example"],"serviceName":"device-virtual","profileName":"Random-Float-Device","autoEvents":[{"interval":"30s","onChange":false,"sourceName":"Float32"},{"interval":"30s","onChange":false,"sourceName":"Float64"}],"protocols":{"other":{"Address":"device-virtual-float-01","Protocol":"300"}}}}`

	DeviceCreateSuccess = `[{"apiVersion":"v3","statusCode":201,"id":"2fff4f1a-7110-442f-b347-9f896338ba57"}]`
	DeviceCreateFail    = `[{"apiVersion":"v3","message":"device name test-Random-Float-Device already exists","statusCode":409}]`

	DeviceDeleteSuccess = `{"apiVersion":"v3","statusCode":200}`
	DeviceDeleteFail    = `{"apiVersion":"v3","message":"fail to query device by name test-Random-Float-Device","statusCode":404}`

	DeviceCoreCommands = `{"apiVersion":"v3","statusCode":200,"deviceCoreCommand":{"deviceName":"Random-Float-Device","profileName":"Random-Float-Device","coreCommands":[{"name":"WriteFloat32ArrayValue","set":true,"path":"/api/v3/device/name/Random-Float-Device/WriteFloat32ArrayValue","url":"http://edgex-core-command:59882","parameters":[{"resourceName":"Float32Array","valueType":"Float32Array"},{"resourceName":"EnableRandomization_Float32Array","valueType":"Bool"}]},{"name":"WriteFloat64ArrayValue","set":true,"path":"/api/v3/device/name/Random-Float-Device/WriteFloat64ArrayValue","url":"http://edgex-core-command:59882","parameters":[{"resourceName":"Float64Array","valueType":"Float64Array"},{"resourceName":"EnableRandomization_Float64Array","valueType":"Bool"}]},{"name":"Float32","get":true,"set":true,"path":"/api/v3/device/name/Random-Float-Device/Float32","url":"http://edgex-core-command:59882","parameters":[{"resourceName":"Float32","valueType":"Float32"}]},{"name":"Float64","get":true,"set":true,"path":"/api/v3/device/name/Random-Float-Device/Float64","url":"http://edgex-core-command:59882","parameters":[{"resourceName":"Float64","valueType":"Float64"}]},{"name":"Float32Array","get":true,"set":true,"path":"/api/v3/device/name/Random-Float-Device/Float32Array","url":"http://edgex-core-command:59882","parameters":[{"resourceName":"Float32Array","valueType":"Float32Array"}]},{"name":"Float64Array","get":true,"set":true,"path":"/api/v3/device/name/Random-Float-Device/Float64Array","url":"http://edgex-core-command:59882","parameters":[{"resourceName":"Float64Array","valueType":"Float64Array"}]},{"name":"WriteFloat32Value","set":true,"path":"/api/v3/device/name/Random-Float-Device/WriteFloat32Value","url":"http://edgex-core-command:59882","parameters":[{"resourceName":"Float32","valueType":"Float32"},{"resourceName":"EnableRandomization_Float32","valueType":"Bool"}]},{"name":"WriteFloat64Value","set":true,"path":"/api/v3/device/name/Random-Float-Device/WriteFloat64Value","url":"http://edgex-core-command:59882","parameters":[{"resourceName":"Float64","valueType":"Float64"},{"resourceName":"EnableRandomization_Float64","valueType":"Bool"}]}]}}`
	DeviceCommandResp  = `{"apiVersion":"v3","statusCode":200,"event":{"apiVersion":"v3","id":"095090e4-de39-45a1-a0fa-18bc340104e6","deviceName":"Random-Float-Device","profileName":"Random-Float-Device","sourceName":"Float32","origin":1661851070562067780,"readings":[{"id":"972bf6be-3b01-49fc-b211-a43ed51d207d","origin":1661851070562067780,"deviceName":"Random-Float-Device","resourceName":"Float32","profileName":"Random-Float-Device","valueType":"Float32","value":"-2.038811e+38"}]}}`

	DeviceUpdateSuccess = `[{"apiVersion":"v3","statusCode":200}] `

	DeviceUpdateProperty = `{"apiVersion":"v3","statusCode":200}`

	DeviceSystemEvent = `{"apiVersion":"v3","type":"device","action":"add","source":"core-metadata","owner":"device-onvif-camera","tags":{"device-profile":"onvif-camera"},"details":{"id":"TestUUID","name":"My-Camera-Device","serviceName":"device-onvif-camera","profileName":"onvif-camera","protocols":{"Onvif":{"Address":"192.168.12.123","Port":"80"}}},"timestamp":1639279435789056000}`
)

var deviceClient = NewEdgexDeviceClient("edgex-core-metadata:59881", "edgex-core-command:59882")

func Test_Get(t *testing.T) {
	httpmock.ActivateNonDefault(deviceClient.Client.GetClient())
	defer httpmock.DeactivateAndReset()
	httpmock.RegisterResponder("GET", "http://edgex-core-metadata:59881/api/v3/device/name/Random-Float-Device",
		httpmock.NewStringResponder(200, DeviceMetadata))

	device, err := deviceClient.Get(context.TODO(), "Random-Float-Device", clients.GetOptions{Namespace: "default"})
	assert.Nil(t, err)

	assert.Equal(t, "Random-Float-Device", device.Spec.Profile)
}

func Test_List(t *testing.T) {

	httpmock.ActivateNonDefault(deviceClient.Client.GetClient())
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("GET", "http://edgex-core-metadata:59881/api/v3/device/all?limit=-1",
		httpmock.NewStringResponder(200, DeviceListMetadata))

	devices, err := deviceClient.List(context.TODO(), clients.ListOptions{Namespace: "default"})
	assert.Nil(t, err)

	assert.Equal(t, len(devices), 5)
}

func Test_Create(t *testing.T) {
	httpmock.ActivateNonDefault(deviceClient.Client.GetClient())
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("POST", "http://edgex-core-metadata:59881/api/v3/device",
		httpmock.NewStringResponder(207, DeviceCreateSuccess))

	var resp edgex_resp.DeviceResponse

	err := json.Unmarshal([]byte(DeviceMetadata), &resp)
	assert.Nil(t, err)

	device := toKubeDevice(resp.Device, "default")
	device.Name = "test-Random-Float-Device"

	create, err := deviceClient.Create(context.TODO(), &device, clients.CreateOptions{})
	assert.Nil(t, err)

	assert.Equal(t, "test-Random-Float-Device", create.Name)

	httpmock.RegisterResponder("POST", "http://edgex-core-metadata:59881/api/v3/device",
		httpmock.NewStringResponder(207, DeviceCreateFail))

	create, err = deviceClient.Create(context.TODO(), &device, clients.CreateOptions{})
	assert.NotNil(t, err)
	assert.Nil(t, create)
}

func Test_Delete(t *testing.T) {
	httpmock.ActivateNonDefault(deviceClient.Client.GetClient())
	defer httpmock.DeactivateAndReset()
	httpmock.RegisterResponder("DELETE", "http://edgex-core-metadata:59881/api/v3/device/name/test-Random-Float-Device",
		httpmock.NewStringResponder(200, DeviceDeleteSuccess))

	err := deviceClient.Delete(context.TODO(), "test-Random-Float-Device", clients.DeleteOptions{})
	assert.Nil(t, err)

	httpmock.RegisterResponder("DELETE", "http://edgex-core-metadata:59881/api/v3/device/name/test-Random-Float-Device",
		httpmock.NewStringResponder(404, DeviceDeleteFail))

	err = deviceClient.Delete(context.TODO(), "test-Random-Float-Device", clients.DeleteOptions{})
	assert.NotNil(t, err)
}

func Test_GetPropertyState(t *testing.T) {

	httpmock.ActivateNonDefault(deviceClient.Client.GetClient())
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("GET", "http://edgex-core-command:59882/api/v3/device/name/Random-Float-Device",
		httpmock.NewStringResponder(200, DeviceCoreCommands))
	httpmock.RegisterResponder("GET", "http://edgex-core-command:59882/api/v3/device/name/Random-Float-Device/Float32",
		httpmock.NewStringResponder(200, DeviceCommandResp))

	var resp edgex_resp.DeviceResponse

	err := json.Unmarshal([]byte(DeviceMetadata), &resp)
	assert.Nil(t, err)

	device := toKubeDevice(resp.Device, "default")

	_, err = deviceClient.GetPropertyState(context.TODO(), "Float32", &device, clients.GetOptions{})
	assert.Nil(t, err)
}

func Test_ListPropertiesState(t *testing.T) {
	httpmock.ActivateNonDefault(deviceClient.Client.GetClient())
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("GET", "http://edgex-core-command:59882/api/v3/device/name/Random-Float-Device",
		httpmock.NewStringResponder(200, DeviceCoreCommands))

	var resp edgex_resp.DeviceResponse

	err := json.Unmarshal([]byte(DeviceMetadata), &resp)
	assert.Nil(t, err)

	device := toKubeDevice(resp.Device, "default")

	_, _, err = deviceClient.ListPropertiesState(context.TODO(), &device, clients.ListOptions{})
	assert.Nil(t, err)
}

func Test_UpdateDevice(t *testing.T) {
	httpmock.ActivateNonDefault(deviceClient.Client.GetClient())
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("PATCH", "http://edgex-core-metadata:59881/api/v3/device",
		httpmock.NewStringResponder(207, DeviceUpdateSuccess))

	var resp edgex_resp.DeviceResponse

	err := json.Unmarshal([]byte(DeviceMetadata), &resp)
	assert.Nil(t, err)

	device := toKubeDevice(resp.Device, "default")
	device.Spec.AdminState = "LOCKED"

	_, err = deviceClient.Update(context.TODO(), &device, clients.UpdateOptions{})
	assert.Nil(t, err)
}

func Test_UpdatePropertyState(t *testing.T) {
	httpmock.ActivateNonDefault(deviceClient.Client.GetClient())
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("GET", "http://edgex-core-command:59882/api/v3/device/name/Random-Float-Device",
		httpmock.NewStringResponder(200, DeviceCoreCommands))

	httpmock.RegisterResponder("PUT", "http://edgex-core-command:59882/api/v3/device/name/Random-Float-Device/Float32",
		httpmock.NewStringResponder(200, DeviceUpdateSuccess))
	var resp edgex_resp.DeviceResponse
	err := json.Unmarshal([]byte(DeviceMetadata), &resp)
	assert.Nil(t, err)

	device := toKubeDevice(resp.Device, "default")
	device.Spec.DeviceProperties = map[string]iotv1alpha1.DesiredPropertyState{
		"Float32": {
			Name:         "Float32",
			DesiredValue: "66.66",
		},
	}

	err = deviceClient.UpdatePropertyState(context.TODO(), "Float32", &device, clients.UpdateOptions{})
	assert.Nil(t, err)
}

func Test_ConvertDeviceSystemEvents(t *testing.T) {
	dse := dtos.SystemEvent{}
	err := json.Unmarshal([]byte(DeviceSystemEvent), &dse)
	if err != nil {
		return
	}

	device, err := deviceClient.Convert(context.TODO(), dse, clients.GetOptions{Namespace: "default"})
	assert.Nil(t, err)
	assert.Equal(t, "my-camera-device", device.Name)
	assert.Equal(t, "device-onvif-camera", device.Spec.Service)
}
