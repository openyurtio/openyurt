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
	"fmt"
	"strings"

	"github.com/edgexfoundry/go-mod-core-contracts/v3/dtos"
	"github.com/edgexfoundry/go-mod-core-contracts/v3/dtos/common"
	"github.com/edgexfoundry/go-mod-core-contracts/v3/dtos/requests"
	"github.com/edgexfoundry/go-mod-core-contracts/v3/models"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	iotv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/iot/v1alpha1"
	util "github.com/openyurtio/openyurt/pkg/yurtiotdock/controllers/util"
)

const (
	EdgeXObjectName     = "yurt-iot-dock/edgex-object.name"
	DeviceServicePath   = "/api/v3/deviceservice"
	DeviceProfilePath   = "/api/v3/deviceprofile"
	DevicePath          = "/api/v3/device"
	CommandResponsePath = "/api/v3/device"

	APIVersionV3 = "v3"
)

type ClientURL struct {
	Host string
	Port int
}

func getEdgeXName(provider metav1.Object) string {
	var actualDeviceName string
	if _, ok := provider.GetLabels()[EdgeXObjectName]; ok {
		actualDeviceName = provider.GetLabels()[EdgeXObjectName]
	} else {
		actualDeviceName = provider.GetName()
	}
	return actualDeviceName
}

func toEdgexDeviceService(ds *iotv1alpha1.DeviceService) dtos.DeviceService {
	return dtos.DeviceService{
		Description: ds.Spec.Description,
		Name:        getEdgeXName(ds),
		Labels:      ds.Spec.Labels,
		AdminState:  string(ds.Spec.AdminState),
		BaseAddress: ds.Spec.BaseAddress,
		// TODO: Metric LastConnected / LastReported
	}
}

func toEdgeXDeviceResourceSlice(drs []iotv1alpha1.DeviceResource) []dtos.DeviceResource {
	var ret []dtos.DeviceResource
	for _, dr := range drs {
		ret = append(ret, toEdgeXDeviceResource(dr))
	}
	return ret
}

func toEdgeXDeviceResource(dr iotv1alpha1.DeviceResource) dtos.DeviceResource {
	genericAttrs := make(map[string]interface{})
	for k, v := range dr.Attributes {
		genericAttrs[k] = v
	}

	return dtos.DeviceResource{
		Description: dr.Description,
		Name:        dr.Name,
		// Tag:         dr.Tag,
		Properties: toEdgeXProfileProperty(dr.Properties),
		Attributes: genericAttrs,
	}
}

func toEdgeXProfileProperty(pp iotv1alpha1.ResourceProperties) dtos.ResourceProperties {
	return dtos.ResourceProperties{
		ReadWrite:    pp.ReadWrite,
		Minimum:      util.StrToFloat(pp.Minimum),
		Maximum:      util.StrToFloat(pp.Maximum),
		DefaultValue: pp.DefaultValue,
		Mask:         util.StrToUint(pp.Mask),
		Shift:        util.StrToInt(pp.Shift),
		Scale:        util.StrToFloat(pp.Scale),
		Offset:       util.StrToFloat(pp.Offset),
		Base:         util.StrToFloat(pp.Base),
		Assertion:    pp.Assertion,
		MediaType:    pp.MediaType,
		Units:        pp.Units,
		ValueType:    pp.ValueType,
	}
}

func toKubeDeviceService(ds dtos.DeviceService, namespace string) iotv1alpha1.DeviceService {
	return iotv1alpha1.DeviceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      toKubeName(ds.Name),
			Namespace: namespace,
			Labels: map[string]string{
				EdgeXObjectName: ds.Name,
			},
		},
		Spec: iotv1alpha1.DeviceServiceSpec{
			Description: ds.Description,
			Labels:      ds.Labels,
			AdminState:  iotv1alpha1.AdminState(ds.AdminState),
			BaseAddress: ds.BaseAddress,
		},
		Status: iotv1alpha1.DeviceServiceStatus{
			EdgeId:     ds.Id,
			AdminState: iotv1alpha1.AdminState(ds.AdminState),
			// TODO: Metric LastConnected / LastReported
		},
	}
}

func toEdgeXDevice(d *iotv1alpha1.Device) dtos.Device {
	md := dtos.Device{
		Description:    d.Spec.Description,
		Name:           getEdgeXName(d),
		AdminState:     string(toEdgeXAdminState(d.Spec.AdminState)),
		OperatingState: string(toEdgeXOperatingState(d.Spec.OperatingState)),
		Protocols:      toEdgeXProtocols(d.Spec.Protocols),
		// TODO: Metric LastConnected / LastReported
		Labels:      d.Spec.Labels,
		Location:    d.Spec.Location,
		ServiceName: d.Spec.Service,
		ProfileName: d.Spec.Profile,
	}
	if d.Status.EdgeId != "" {
		md.Id = d.Status.EdgeId
	}
	return md
}

func toEdgeXUpdateDevice(d *iotv1alpha1.Device) dtos.UpdateDevice {
	adminState := string(toEdgeXAdminState(d.Spec.AdminState))
	operationState := string(toEdgeXOperatingState(d.Spec.OperatingState))
	md := dtos.UpdateDevice{
		Description:    &d.Spec.Description,
		Name:           &d.Name,
		AdminState:     &adminState,
		OperatingState: &operationState,
		Protocols:      toEdgeXProtocols(d.Spec.Protocols),
		Labels:         d.Spec.Labels,
		Location:       d.Spec.Location,
		ServiceName:    &d.Spec.Service,
		ProfileName:    &d.Spec.Profile,
		// TODO: Metric LastConnected / LastReported
	}
	if d.Status.EdgeId != "" {
		md.Id = &d.Status.EdgeId
	}
	return md
}

func toEdgeXProtocols(
	pps map[string]iotv1alpha1.ProtocolProperties) map[string]dtos.ProtocolProperties {
	ret := make(map[string]dtos.ProtocolProperties, len(pps))
	for k, v := range pps {
		propMap := make(map[string]interface{})
		for key, value := range v {
			propMap[key] = value
		}
		ret[k] = dtos.ProtocolProperties(propMap)
	}
	return ret
}

func toEdgeXAdminState(as iotv1alpha1.AdminState) models.AdminState {
	if as == iotv1alpha1.Locked {
		return models.Locked
	}
	return models.Unlocked
}

func toEdgeXOperatingState(os iotv1alpha1.OperatingState) models.OperatingState {
	if os == iotv1alpha1.Up {
		return models.Up
	} else if os == iotv1alpha1.Down {
		return models.Down
	}
	return models.Unknown
}

// toKubeDevice serialize the EdgeX Device to the corresponding Kubernetes Device
func toKubeDevice(ed dtos.Device, namespace string) iotv1alpha1.Device {
	var loc string
	if ed.Location != nil {
		loc = ed.Location.(string)
	}
	return iotv1alpha1.Device{
		ObjectMeta: metav1.ObjectMeta{
			Name:      toKubeName(ed.Name),
			Namespace: namespace,
			Labels: map[string]string{
				EdgeXObjectName: ed.Name,
			},
		},
		Spec: iotv1alpha1.DeviceSpec{
			Description:    ed.Description,
			AdminState:     iotv1alpha1.AdminState(ed.AdminState),
			OperatingState: iotv1alpha1.OperatingState(ed.OperatingState),
			Protocols:      toKubeProtocols(ed.Protocols),
			Labels:         ed.Labels,
			Location:       loc,
			Service:        ed.ServiceName,
			Profile:        ed.ProfileName,
			// TODO: Notify
		},
		Status: iotv1alpha1.DeviceStatus{
			// TODO: Metric LastConnected / LastReported
			Synced:         true,
			EdgeId:         ed.Id,
			AdminState:     iotv1alpha1.AdminState(ed.AdminState),
			OperatingState: iotv1alpha1.OperatingState(ed.OperatingState),
		},
	}
}

// toKubeProtocols serialize the EdgeX ProtocolProperties to the corresponding
// Kubernetes OperatingState
func toKubeProtocols(
	eps map[string]dtos.ProtocolProperties) map[string]iotv1alpha1.ProtocolProperties {
	ret := map[string]iotv1alpha1.ProtocolProperties{}
	for k, v := range eps {
		propMap := make(map[string]string)
		for key, value := range v {
			switch asserted := value.(type) {
			case string:
				propMap[key] = asserted
				continue
			case int:
				propMap[key] = fmt.Sprintf("%d", asserted)
				continue
			case float64:
				propMap[key] = fmt.Sprintf("%f", asserted)
				continue
			case fmt.Stringer:
				propMap[key] = asserted.String()
				continue
			}
		}
		ret[k] = iotv1alpha1.ProtocolProperties(propMap)
	}
	return ret
}

// toKubeDeviceProfile create DeviceProfile in cloud according to devicProfile in edge
func toKubeDeviceProfile(dp *dtos.DeviceProfile, namespace string) iotv1alpha1.DeviceProfile {
	return iotv1alpha1.DeviceProfile{
		ObjectMeta: metav1.ObjectMeta{
			Name:      toKubeName(dp.Name),
			Namespace: namespace,
			Labels: map[string]string{
				EdgeXObjectName: dp.Name,
			},
		},
		Spec: iotv1alpha1.DeviceProfileSpec{
			Description:     dp.Description,
			Manufacturer:    dp.Manufacturer,
			Model:           dp.Model,
			Labels:          dp.Labels,
			DeviceResources: toKubeDeviceResources(dp.DeviceResources),
			DeviceCommands:  toKubeDeviceCommand(dp.DeviceCommands),
		},
		Status: iotv1alpha1.DeviceProfileStatus{
			EdgeId: dp.Id,
			Synced: true,
		},
	}
}

func toKubeDeviceCommand(dcs []dtos.DeviceCommand) []iotv1alpha1.DeviceCommand {
	var ret []iotv1alpha1.DeviceCommand
	for _, dc := range dcs {
		ret = append(ret, iotv1alpha1.DeviceCommand{
			Name:               dc.Name,
			ReadWrite:          dc.ReadWrite,
			IsHidden:           dc.IsHidden,
			ResourceOperations: toKubeResourceOperations(dc.ResourceOperations),
		})
	}
	return ret
}

func toEdgeXDeviceCommand(dcs []iotv1alpha1.DeviceCommand) []dtos.DeviceCommand {
	var ret []dtos.DeviceCommand
	for _, dc := range dcs {
		ret = append(ret, dtos.DeviceCommand{
			Name:               dc.Name,
			ReadWrite:          dc.ReadWrite,
			IsHidden:           dc.IsHidden,
			ResourceOperations: toEdgeXResourceOperations(dc.ResourceOperations),
		})
	}
	return ret
}

func toKubeResourceOperations(ros []dtos.ResourceOperation) []iotv1alpha1.ResourceOperation {
	var ret []iotv1alpha1.ResourceOperation
	for _, ro := range ros {
		ret = append(ret, iotv1alpha1.ResourceOperation{
			DeviceResource: ro.DeviceResource,
			Mappings:       ro.Mappings,
			DefaultValue:   ro.DefaultValue,
		})
	}
	return ret
}

func toEdgeXResourceOperations(ros []iotv1alpha1.ResourceOperation) []dtos.ResourceOperation {
	var ret []dtos.ResourceOperation
	for _, ro := range ros {
		ret = append(ret, dtos.ResourceOperation{
			DeviceResource: ro.DeviceResource,
			Mappings:       ro.Mappings,
			DefaultValue:   ro.DefaultValue,
		})
	}
	return ret
}

func toKubeDeviceResources(drs []dtos.DeviceResource) []iotv1alpha1.DeviceResource {
	var ret []iotv1alpha1.DeviceResource
	for _, dr := range drs {
		ret = append(ret, toKubeDeviceResource(dr))
	}
	return ret
}

func toKubeDeviceResource(dr dtos.DeviceResource) iotv1alpha1.DeviceResource {
	concreteAttrs := make(map[string]string)
	for k, v := range dr.Attributes {
		switch asserted := v.(type) {
		case string:
			concreteAttrs[k] = asserted
			continue
		case int:
			concreteAttrs[k] = fmt.Sprintf("%d", asserted)
			continue
		case float64:
			concreteAttrs[k] = fmt.Sprintf("%f", asserted)
			continue
		case fmt.Stringer:
			concreteAttrs[k] = asserted.String()
			continue
		}
	}

	return iotv1alpha1.DeviceResource{
		Description: dr.Description,
		Name:        dr.Name,
		// Tag:         dr.Tag,
		IsHidden:   dr.IsHidden,
		Properties: toKubeProfileProperty(dr.Properties),
		Attributes: concreteAttrs,
	}
}

func toKubeProfileProperty(rp dtos.ResourceProperties) iotv1alpha1.ResourceProperties {
	return iotv1alpha1.ResourceProperties{
		ValueType:    rp.ValueType,
		ReadWrite:    rp.ReadWrite,
		Minimum:      util.FloatToStr(rp.Minimum),
		Maximum:      util.FloatToStr(rp.Maximum),
		DefaultValue: rp.DefaultValue,
		Mask:         util.UintToStr(rp.Mask),
		Shift:        util.IntToStr(rp.Shift),
		Scale:        util.FloatToStr(rp.Scale),
		Offset:       util.FloatToStr(rp.Offset),
		Base:         util.FloatToStr(rp.Base),
		Assertion:    rp.Assertion,
		MediaType:    rp.MediaType,
		Units:        rp.Units,
	}
}

// toEdgeXDeviceProfile create DeviceProfile in edge according to devicProfile in cloud
func toEdgeXDeviceProfile(dp *iotv1alpha1.DeviceProfile) dtos.DeviceProfile {
	return dtos.DeviceProfile{
		DeviceProfileBasicInfo: dtos.DeviceProfileBasicInfo{
			Description:  dp.Spec.Description,
			Name:         getEdgeXName(dp),
			Manufacturer: dp.Spec.Manufacturer,
			Model:        dp.Spec.Model,
			Labels:       dp.Spec.Labels,
		},
		DeviceResources: toEdgeXDeviceResourceSlice(dp.Spec.DeviceResources),
		DeviceCommands:  toEdgeXDeviceCommand(dp.Spec.DeviceCommands),
	}
}

func makeEdgeXDeviceProfilesRequest(dps []*iotv1alpha1.DeviceProfile) []*requests.DeviceProfileRequest {
	var req []*requests.DeviceProfileRequest
	for _, dp := range dps {
		req = append(req, &requests.DeviceProfileRequest{
			BaseRequest: common.BaseRequest{
				Versionable: common.Versionable{
					ApiVersion: APIVersionV3,
				},
			},
			Profile: toEdgeXDeviceProfile(dp),
		})
	}
	return req
}

func makeEdgeXDeviceUpdateRequest(devs []*iotv1alpha1.Device) []*requests.UpdateDeviceRequest {
	var req []*requests.UpdateDeviceRequest
	for _, dev := range devs {
		req = append(req, &requests.UpdateDeviceRequest{
			BaseRequest: common.BaseRequest{
				Versionable: common.Versionable{
					ApiVersion: APIVersionV3,
				},
			},
			Device: toEdgeXUpdateDevice(dev),
		})
	}
	return req
}

func makeEdgeXDeviceRequest(devs []*iotv1alpha1.Device) []*requests.AddDeviceRequest {
	var req []*requests.AddDeviceRequest
	for _, dev := range devs {
		req = append(req, &requests.AddDeviceRequest{
			BaseRequest: common.BaseRequest{
				Versionable: common.Versionable{
					ApiVersion: APIVersionV3,
				},
			},
			Device: toEdgeXDevice(dev),
		})
	}
	return req
}

func makeEdgeXDeviceService(dss []*iotv1alpha1.DeviceService) []*requests.AddDeviceServiceRequest {
	var req []*requests.AddDeviceServiceRequest
	for _, ds := range dss {
		req = append(req, &requests.AddDeviceServiceRequest{
			BaseRequest: common.BaseRequest{
				Versionable: common.Versionable{
					ApiVersion: APIVersionV3,
				},
			},
			Service: toEdgexDeviceService(ds),
		})
	}
	return req
}

func toKubeName(edgexName string) string {
	return strings.ReplaceAll(strings.ToLower(edgexName), "_", "-")
}
