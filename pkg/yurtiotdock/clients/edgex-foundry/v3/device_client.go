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
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"

	"github.com/edgexfoundry/go-mod-core-contracts/v3/dtos"
	"github.com/edgexfoundry/go-mod-core-contracts/v3/dtos/common"
	edgex_resp "github.com/edgexfoundry/go-mod-core-contracts/v3/dtos/responses"
	"github.com/go-resty/resty/v2"
	"golang.org/x/net/publicsuffix"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/apis/iot/v1alpha1"
	iotv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/iot/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/yurtiotdock/clients"
	devcli "github.com/openyurtio/openyurt/pkg/yurtiotdock/clients"
)

type EdgexDeviceClient struct {
	*resty.Client
	CoreMetaAddr    string
	CoreCommandAddr string
}

func NewEdgexDeviceClient(coreMetaAddr, coreCommandAddr string) *EdgexDeviceClient {
	cookieJar, _ := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	instance := resty.NewWithClient(&http.Client{
		Jar:     cookieJar,
		Timeout: 10 * time.Second,
	})
	return &EdgexDeviceClient{
		Client:          instance,
		CoreMetaAddr:    coreMetaAddr,
		CoreCommandAddr: coreCommandAddr,
	}
}

// Create function sends a POST request to EdgeX to add a new device
func (efc *EdgexDeviceClient) Create(ctx context.Context, device *iotv1alpha1.Device, options clients.CreateOptions) (*iotv1alpha1.Device, error) {
	devs := []*iotv1alpha1.Device{device}
	req := makeEdgeXDeviceRequest(devs)
	klog.V(5).Infof("will add the Device: %s", device.Name)
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	postPath := fmt.Sprintf("http://%s%s", efc.CoreMetaAddr, DevicePath)
	resp, err := efc.R().
		SetBody(reqBody).Post(postPath)
	if err != nil {
		return nil, err
	} else if resp.StatusCode() != http.StatusMultiStatus {
		return nil, fmt.Errorf("create device on edgex foundry failed, the response is : %s", resp.Body())
	}

	var edgexResps []*common.BaseWithIdResponse
	if err = json.Unmarshal(resp.Body(), &edgexResps); err != nil {
		return nil, err
	}
	createdDevice := device.DeepCopy()
	if len(edgexResps) == 1 {
		if edgexResps[0].StatusCode == http.StatusCreated {
			createdDevice.Status.EdgeId = edgexResps[0].Id
			createdDevice.Status.Synced = true
		} else {
			return nil, fmt.Errorf("create device on edgex foundry failed, the response is : %s", resp.Body())
		}
	} else {
		return nil, fmt.Errorf("edgex BaseWithIdResponse count mismatch device cound, the response is : %s", resp.Body())
	}
	return createdDevice, err
}

// Delete function sends a request to EdgeX to delete a device
func (efc *EdgexDeviceClient) Delete(ctx context.Context, name string, options clients.DeleteOptions) error {
	klog.V(5).Infof("will delete the Device: %s", name)
	delURL := fmt.Sprintf("http://%s%s/name/%s", efc.CoreMetaAddr, DevicePath, name)
	resp, err := efc.R().Delete(delURL)
	if err != nil {
		return err
	}
	if resp.StatusCode() != http.StatusOK {
		return errors.New(string(resp.Body()))
	}
	return nil
}

// Update is used to set the admin or operating state of the device by unique name of the device.
// TODO support to update other fields
func (efc *EdgexDeviceClient) Update(ctx context.Context, device *iotv1alpha1.Device, options clients.UpdateOptions) (*iotv1alpha1.Device, error) {
	actualDeviceName := getEdgeXName(device)
	patchURL := fmt.Sprintf("http://%s%s", efc.CoreMetaAddr, DevicePath)
	if device == nil {
		return nil, nil
	}
	devs := []*iotv1alpha1.Device{device}
	req := makeEdgeXDeviceUpdateRequest(devs)
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	rep, err := efc.R().
		SetHeader("Content-Type", "application/json").
		SetBody(reqBody).
		Patch(patchURL)
	if err != nil {
		return nil, err
	} else if rep.StatusCode() != http.StatusMultiStatus {
		return nil, fmt.Errorf("could not update device: %s, get response: %s", actualDeviceName, string(rep.Body()))
	}
	return device, nil
}

// Get is used to query the device information corresponding to the device name
func (efc *EdgexDeviceClient) Get(ctx context.Context, deviceName string, options clients.GetOptions) (*iotv1alpha1.Device, error) {
	klog.V(5).Infof("will get Devices: %s", deviceName)
	var dResp edgex_resp.DeviceResponse
	getURL := fmt.Sprintf("http://%s%s/name/%s", efc.CoreMetaAddr, DevicePath, deviceName)
	resp, err := efc.R().Get(getURL)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() == http.StatusNotFound {
		return nil, fmt.Errorf("Device %s not found", deviceName)
	}
	err = json.Unmarshal(resp.Body(), &dResp)
	if err != nil {
		return nil, err
	}
	device := toKubeDevice(dResp.Device, options.Namespace)
	return &device, err
}

// Convert is used to convert the device information in the systemEvent of messageBus to the device object in the kubernetes cluster
func (cdc *EdgexDeviceClient) Convert(ctx context.Context, systemEvent dtos.SystemEvent, opts devcli.GetOptions) (*v1alpha1.Device, error) {
	dto := dtos.Device{}
	err := systemEvent.DecodeDetails(&dto)
	if err != nil {
		klog.V(3).ErrorS(err, "fail to decode device systemEvent details")
		return nil, err
	}

	device := toKubeDevice(dto, opts.Namespace)
	return &device, nil
}

// List is used to get all device objects on edge platform
// TODO:support label filtering according to options
func (efc *EdgexDeviceClient) List(ctx context.Context, options clients.ListOptions) ([]iotv1alpha1.Device, error) {
	lp := fmt.Sprintf("http://%s%s/all?limit=-1", efc.CoreMetaAddr, DevicePath)
	resp, err := efc.R().EnableTrace().Get(lp)
	if err != nil {
		return nil, err
	}
	var mdResp edgex_resp.MultiDevicesResponse
	if err := json.Unmarshal(resp.Body(), &mdResp); err != nil {
		return nil, err
	}
	var res []iotv1alpha1.Device
	for _, dp := range mdResp.Devices {
		res = append(res, toKubeDevice(dp, options.Namespace))
	}
	return res, nil
}

func (efc *EdgexDeviceClient) GetPropertyState(ctx context.Context, propertyName string, d *iotv1alpha1.Device, options clients.GetOptions) (*iotv1alpha1.ActualPropertyState, error) {
	actualDeviceName := getEdgeXName(d)
	// get the old property from status
	oldAps, exist := d.Status.DeviceProperties[propertyName]
	propertyGetURL := ""
	// 1. query the Get URL of a property
	if !exist || (exist && oldAps.GetURL == "") {
		coreCommands, err := efc.GetCommandResponseByName(actualDeviceName)
		if err != nil {
			return &iotv1alpha1.ActualPropertyState{}, err
		}
		for _, c := range coreCommands {
			if c.Name == propertyName && c.Get {
				propertyGetURL = fmt.Sprintf("%s%s", c.Url, c.Path)
				break
			}
		}
		if propertyGetURL == "" {
			return nil, &clients.NotFoundError{}
		}
	} else {
		propertyGetURL = oldAps.GetURL
	}
	// 2. get the actual property value by the getURL
	actualPropertyState := iotv1alpha1.ActualPropertyState{
		Name:   propertyName,
		GetURL: propertyGetURL,
	}
	if resp, err := efc.getPropertyState(propertyGetURL); err != nil {
		return nil, err
	} else {
		var eResp edgex_resp.EventResponse
		if err := json.Unmarshal(resp.Body(), &eResp); err != nil {
			return nil, err
		}
		actualPropertyState.ActualValue = getPropertyValueFromEvent(propertyName, eResp.Event)
	}
	return &actualPropertyState, nil
}

// getPropertyState returns different error messages according to the status code
func (efc *EdgexDeviceClient) getPropertyState(getURL string) (*resty.Response, error) {
	resp, err := efc.R().Get(getURL)
	if err != nil {
		return resp, err
	}
	if resp.StatusCode() == 400 {
		err = errors.New("request is in an invalid state")
	} else if resp.StatusCode() == 404 {
		err = errors.New("the requested resource does not exist")
	} else if resp.StatusCode() == 423 {
		err = errors.New("the device is locked (AdminState) or down (OperatingState)")
	} else if resp.StatusCode() == 500 {
		err = errors.New("an unexpected error occurred on the server")
	}
	return resp, err
}

func (efc *EdgexDeviceClient) UpdatePropertyState(ctx context.Context, propertyName string, d *iotv1alpha1.Device, options clients.UpdateOptions) error {
	// Get the actual device name
	acturalDeviceName := getEdgeXName(d)

	dps := d.Spec.DeviceProperties[propertyName]
	parameterName := dps.Name
	if dps.PutURL == "" {
		putCmd, err := efc.getPropertyPut(acturalDeviceName, dps.Name)
		if err != nil {
			return err
		}
		dps.PutURL = fmt.Sprintf("%s%s", putCmd.Url, putCmd.Path)
		if len(putCmd.Parameters) == 1 {
			parameterName = putCmd.Parameters[0].ResourceName
		}
	}
	// set the device property to desired state
	bodyMap := make(map[string]string)
	bodyMap[parameterName] = dps.DesiredValue
	body, _ := json.Marshal(bodyMap)
	klog.V(5).Infof("setting the property to desired value", "propertyName", parameterName, "desiredValue", string(body))
	rep, err := efc.R().
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		Put(dps.PutURL)
	if err != nil {
		return err
	} else if rep.StatusCode() != http.StatusOK {
		return fmt.Errorf("could not set property: %s, get response: %s", dps.Name, string(rep.Body()))
	} else if rep.Body() != nil {
		// If the parameters are illegal, such as out of range, the 200 status code is also returned, but the description appears in the body
		a := string(rep.Body())
		if strings.Contains(a, "execWriteCmd") {
			return fmt.Errorf("could not set property: %s, get response: %s", dps.Name, string(rep.Body()))
		}
	}
	return nil
}

// Gets the models.Put from edgex foundry which is used to set the device property's value
func (efc *EdgexDeviceClient) getPropertyPut(deviceName, cmdName string) (dtos.CoreCommand, error) {
	coreCommands, err := efc.GetCommandResponseByName(deviceName)
	if err != nil {
		return dtos.CoreCommand{}, err
	}
	for _, c := range coreCommands {
		if cmdName == c.Name && c.Set {
			return c, nil
		}
	}
	return dtos.CoreCommand{}, errors.New("corresponding command is not found")
}

// ListPropertiesState gets all the actual property information about a device
func (efc *EdgexDeviceClient) ListPropertiesState(ctx context.Context, device *iotv1alpha1.Device, options clients.ListOptions) (map[string]iotv1alpha1.DesiredPropertyState, map[string]iotv1alpha1.ActualPropertyState, error) {
	actualDeviceName := getEdgeXName(device)

	dpsm := map[string]iotv1alpha1.DesiredPropertyState{}
	apsm := map[string]iotv1alpha1.ActualPropertyState{}
	coreCommands, err := efc.GetCommandResponseByName(actualDeviceName)
	if err != nil {
		return dpsm, apsm, err
	}

	for _, c := range coreCommands {
		// DesiredPropertyState only store the basic information and does not set DesiredValue
		if c.Get {
			getURL := fmt.Sprintf("%s%s", c.Url, c.Path)
			aps, ok := apsm[c.Name]
			if ok {
				aps.GetURL = getURL
			} else {
				aps = iotv1alpha1.ActualPropertyState{Name: c.Name, GetURL: getURL}
			}
			apsm[c.Name] = aps
			resp, err := efc.getPropertyState(getURL)
			if err != nil {
				klog.V(5).ErrorS(err, "getPropertyState failed", "propertyName", c.Name, "deviceName", actualDeviceName)
			} else {
				var eResp edgex_resp.EventResponse
				if err := json.Unmarshal(resp.Body(), &eResp); err != nil {
					klog.V(5).ErrorS(err, "could not decode the response ", "response", resp)
					continue
				}
				event := eResp.Event
				readingName := c.Name
				expectParams := c.Parameters
				if len(expectParams) == 1 {
					readingName = expectParams[0].ResourceName
				}
				klog.V(5).Infof("get reading name %s for command %s of device %s", readingName, c.Name, device.Name)
				actualValue := getPropertyValueFromEvent(readingName, event)
				aps.ActualValue = actualValue
				apsm[c.Name] = aps
			}
		}
	}
	return dpsm, apsm, nil
}

// The actual property value is resolved from the returned event
func getPropertyValueFromEvent(resName string, event dtos.Event) string {
	actualValue := ""
	for _, r := range event.Readings {
		if resName == r.ResourceName {
			if r.SimpleReading.Value != "" {
				actualValue = r.SimpleReading.Value
			} else if len(r.BinaryReading.BinaryValue) != 0 {
				// TODO: how to demonstrate binary data
				actualValue = fmt.Sprintf("%s:%s", r.BinaryReading.MediaType, "blob value")
			} else if r.ObjectReading.ObjectValue != nil {
				serializedBytes, _ := json.Marshal(r.ObjectReading.ObjectValue)
				actualValue = string(serializedBytes)
			}
			break
		}
	}
	return actualValue
}

// GetCommandResponseByName gets all commands supported by the device
func (efc *EdgexDeviceClient) GetCommandResponseByName(deviceName string) ([]dtos.CoreCommand, error) {
	klog.V(5).Infof("will get CommandResponses of device: %s", deviceName)

	var dcr edgex_resp.DeviceCoreCommandResponse
	getURL := fmt.Sprintf("http://%s%s/name/%s", efc.CoreCommandAddr, CommandResponsePath, deviceName)

	resp, err := efc.R().Get(getURL)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() == http.StatusNotFound {
		return nil, errors.New("Item not found")
	}
	err = json.Unmarshal(resp.Body(), &dcr)
	if err != nil {
		return nil, err
	}
	return dcr.DeviceCoreCommand.CoreCommands, nil
}
