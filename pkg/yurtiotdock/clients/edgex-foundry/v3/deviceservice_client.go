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
	"net/http"

	"github.com/edgexfoundry/go-mod-core-contracts/v3/dtos"
	"github.com/edgexfoundry/go-mod-core-contracts/v3/dtos/common"
	"github.com/edgexfoundry/go-mod-core-contracts/v3/dtos/responses"
	"github.com/go-resty/resty/v2"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/apis/iot/v1alpha1"
	devcli "github.com/openyurtio/openyurt/pkg/yurtiotdock/clients"
	edgeCli "github.com/openyurtio/openyurt/pkg/yurtiotdock/clients"
)

type EdgexDeviceServiceClient struct {
	*resty.Client
	CoreMetaAddr string
}

func NewEdgexDeviceServiceClient(coreMetaAddr string) *EdgexDeviceServiceClient {
	return &EdgexDeviceServiceClient{
		Client:       resty.New(),
		CoreMetaAddr: coreMetaAddr,
	}
}

// Create function sends a POST request to EdgeX to add a new deviceService
func (eds *EdgexDeviceServiceClient) Create(ctx context.Context, deviceService *v1alpha1.DeviceService, options edgeCli.CreateOptions) (*v1alpha1.DeviceService, error) {
	dss := []*v1alpha1.DeviceService{deviceService}
	req := makeEdgeXDeviceService(dss)
	klog.V(5).InfoS("will add the DeviceServices", "DeviceService", deviceService.Name)
	jsonBody, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	postPath := fmt.Sprintf("http://%s%s", eds.CoreMetaAddr, DeviceServicePath)
	resp, err := eds.R().
		SetBody(jsonBody).Post(postPath)
	if err != nil {
		return nil, err
	} else if resp.StatusCode() != http.StatusMultiStatus {
		return nil, fmt.Errorf("create DeviceService on edgex foundry failed, the response is : %s", resp.Body())
	}

	var edgexResps []*common.BaseWithIdResponse
	if err = json.Unmarshal(resp.Body(), &edgexResps); err != nil {
		return nil, err
	}
	createdDeviceService := deviceService.DeepCopy()
	if len(edgexResps) == 1 {
		if edgexResps[0].StatusCode == http.StatusCreated {
			createdDeviceService.Status.EdgeId = edgexResps[0].Id
			createdDeviceService.Status.Synced = true
		} else {
			return nil, fmt.Errorf("create DeviceService on edgex foundry failed, the response is : %s", resp.Body())
		}
	} else {
		return nil, fmt.Errorf("edgex BaseWithIdResponse count mismatch DeviceService count, the response is : %s", resp.Body())
	}
	return createdDeviceService, err
}

// Convert is used to convert the device service information in the systemEvent of messageBus to the device service object in the kubernetes cluster
func (cdc *EdgexDeviceServiceClient) Convert(ctx context.Context, systemEvent dtos.SystemEvent, opts devcli.GetOptions) (*v1alpha1.DeviceService, error) {
	dto := dtos.DeviceService{}
	err := systemEvent.DecodeDetails(&dto)
	if err != nil {
		klog.V(3).ErrorS(err, "fail to decode deviceservice systemEvent details")
		return nil, err
	}

	deeviceService := toKubeDeviceService(dto, opts.Namespace)
	return &deeviceService, nil
}

// Delete function sends a request to EdgeX to delete a deviceService
func (eds *EdgexDeviceServiceClient) Delete(ctx context.Context, name string, option edgeCli.DeleteOptions) error {
	klog.V(5).InfoS("will delete the DeviceService", "DeviceService", name)
	delURL := fmt.Sprintf("http://%s%s/name/%s", eds.CoreMetaAddr, DeviceServicePath, name)
	resp, err := eds.R().Delete(delURL)
	if err != nil {
		return err
	}
	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("delete edgex deviceservice err: %s", string(resp.Body()))
	}
	return nil
}

// Update is used to set the admin or operating state of the deviceService by unique name of the deviceService.
// TODO support to update other fields
func (eds *EdgexDeviceServiceClient) Update(ctx context.Context, ds *v1alpha1.DeviceService, options edgeCli.UpdateOptions) (*v1alpha1.DeviceService, error) {
	patchURL := fmt.Sprintf("http://%s%s", eds.CoreMetaAddr, DeviceServicePath)
	if ds == nil {
		return nil, nil
	}

	if ds.Status.EdgeId == "" {
		return nil, fmt.Errorf("could not update deviceservice %s with empty edgex id", ds.Name)
	}
	edgeDs := toEdgexDeviceService(ds)
	edgeDs.Id = ds.Status.EdgeId
	dsJson, err := json.Marshal(&edgeDs)
	if err != nil {
		return nil, err
	}
	resp, err := eds.R().
		SetBody(dsJson).Patch(patchURL)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode() == http.StatusOK || resp.StatusCode() == http.StatusMultiStatus {
		return ds, nil
	} else {
		return nil, fmt.Errorf("request to patch deviceservice failed, errcode:%d", resp.StatusCode())
	}
}

// Get is used to query the deviceService information corresponding to the deviceService name
func (eds *EdgexDeviceServiceClient) Get(ctx context.Context, name string, options edgeCli.GetOptions) (*v1alpha1.DeviceService, error) {
	klog.V(5).InfoS("will get DeviceServices", "DeviceService", name)
	var dsResp responses.DeviceServiceResponse
	getURL := fmt.Sprintf("http://%s%s/name/%s", eds.CoreMetaAddr, DeviceServicePath, name)
	resp, err := eds.R().Get(getURL)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() == http.StatusNotFound {
		return nil, fmt.Errorf("deviceservice %s not found", name)
	}
	err = json.Unmarshal(resp.Body(), &dsResp)
	if err != nil {
		return nil, err
	}
	ds := toKubeDeviceService(dsResp.Service, options.Namespace)
	return &ds, nil
}

// List is used to get all deviceService objects on edge platform
// The Hanoi version currently supports only a single label and does not support other filters
func (eds *EdgexDeviceServiceClient) List(ctx context.Context, options edgeCli.ListOptions) ([]v1alpha1.DeviceService, error) {
	klog.V(5).Info("will list DeviceServices")
	lp := fmt.Sprintf("http://%s%s/all?limit=-1", eds.CoreMetaAddr, DeviceServicePath)
	resp, err := eds.R().
		EnableTrace().
		Get(lp)
	if err != nil {
		return nil, err
	}
	var mdsResponse responses.MultiDeviceServicesResponse
	if err := json.Unmarshal(resp.Body(), &mdsResponse); err != nil {
		return nil, err
	}
	var res []v1alpha1.DeviceService
	for _, ds := range mdsResponse.Services {
		res = append(res, toKubeDeviceService(ds, options.Namespace))
	}
	return res, nil
}
