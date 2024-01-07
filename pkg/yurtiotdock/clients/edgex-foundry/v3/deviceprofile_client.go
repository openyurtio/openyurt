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
)

type EdgexDeviceProfile struct {
	*resty.Client
	CoreMetaAddr string
}

func NewEdgexDeviceProfile(coreMetaAddr string) *EdgexDeviceProfile {
	return &EdgexDeviceProfile{
		Client:       resty.New(),
		CoreMetaAddr: coreMetaAddr,
	}
}

// TODO: support label filtering
func getListDeviceProfileURL(address string, opts devcli.ListOptions) (string, error) {
	url := fmt.Sprintf("http://%s%s/all?limit=-1", address, DeviceProfilePath)
	return url, nil
}

func (cdc *EdgexDeviceProfile) List(ctx context.Context, opts devcli.ListOptions) ([]v1alpha1.DeviceProfile, error) {
	klog.V(5).Info("will list DeviceProfiles")
	lp, err := getListDeviceProfileURL(cdc.CoreMetaAddr, opts)
	if err != nil {
		return nil, err
	}
	resp, err := cdc.R().EnableTrace().Get(lp)
	if err != nil {
		return nil, err
	}
	var mdpResp responses.MultiDeviceProfilesResponse
	if err := json.Unmarshal(resp.Body(), &mdpResp); err != nil {
		return nil, err
	}
	var deviceProfiles []v1alpha1.DeviceProfile
	for _, dp := range mdpResp.Profiles {
		deviceProfiles = append(deviceProfiles, toKubeDeviceProfile(&dp, opts.Namespace))
	}
	return deviceProfiles, nil
}

func (cdc *EdgexDeviceProfile) Get(ctx context.Context, name string, opts devcli.GetOptions) (*v1alpha1.DeviceProfile, error) {
	klog.V(5).Infof("will get DeviceProfiles: %s", name)
	var dpResp responses.DeviceProfileResponse
	getURL := fmt.Sprintf("http://%s%s/name/%s", cdc.CoreMetaAddr, DeviceProfilePath, name)
	resp, err := cdc.R().Get(getURL)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() == http.StatusNotFound {
		return nil, fmt.Errorf("DeviceProfile %s not found", name)
	}
	if err = json.Unmarshal(resp.Body(), &dpResp); err != nil {
		return nil, err
	}
	kubedp := toKubeDeviceProfile(&dpResp.Profile, opts.Namespace)
	return &kubedp, nil
}

// Convert is used to convert the device profile information in the systemEvent of messageBus to the device profile object in the kubernetes cluster
func (cdc *EdgexDeviceProfile) Convert(ctx context.Context, systemEvent dtos.SystemEvent, opts devcli.GetOptions) (*v1alpha1.DeviceProfile, error) {
	dto := dtos.DeviceProfile{}
	err := systemEvent.DecodeDetails(&dto)
	if err != nil {
		klog.V(3).ErrorS(err, "fail to decode deviceprofile systemEvent details")
		return nil, err
	}

	deeviceProfile := toKubeDeviceProfile(&dto, opts.Namespace)
	return &deeviceProfile, nil
}

func (cdc *EdgexDeviceProfile) Create(ctx context.Context, deviceProfile *v1alpha1.DeviceProfile, opts devcli.CreateOptions) (*v1alpha1.DeviceProfile, error) {
	dps := []*v1alpha1.DeviceProfile{deviceProfile}
	req := makeEdgeXDeviceProfilesRequest(dps)
	klog.V(5).Infof("will add the DeviceProfile: %s", deviceProfile.Name)
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	postURL := fmt.Sprintf("http://%s%s", cdc.CoreMetaAddr, DeviceProfilePath)
	resp, err := cdc.R().SetBody(reqBody).Post(postURL)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusMultiStatus {
		return nil, fmt.Errorf("create edgex deviceProfile err: %s", string(resp.Body())) // 假定 resp.Body() 存了 msg 信息
	}
	var edgexResps []*common.BaseWithIdResponse
	if err = json.Unmarshal(resp.Body(), &edgexResps); err != nil {
		return nil, err
	}
	createdDeviceProfile := deviceProfile.DeepCopy()
	if len(edgexResps) == 1 {
		if edgexResps[0].StatusCode == http.StatusCreated {
			createdDeviceProfile.Status.EdgeId = edgexResps[0].Id
			createdDeviceProfile.Status.Synced = true
		} else {
			return nil, fmt.Errorf("create deviceprofile on edgex foundry failed, the response is : %s", resp.Body())
		}
	} else {
		return nil, fmt.Errorf("edgex BaseWithIdResponse count mismatch DeviceProfile count, the response is : %s", resp.Body())
	}
	return createdDeviceProfile, err
}

// TODO: edgex does not support update DeviceProfile
func (cdc *EdgexDeviceProfile) Update(ctx context.Context, deviceProfile *v1alpha1.DeviceProfile, opts devcli.UpdateOptions) (*v1alpha1.DeviceProfile, error) {
	return nil, nil
}

func (cdc *EdgexDeviceProfile) Delete(ctx context.Context, name string, opts devcli.DeleteOptions) error {
	klog.V(5).Infof("will delete the DeviceProfile: %s", name)
	delURL := fmt.Sprintf("http://%s%s/name/%s", cdc.CoreMetaAddr, DeviceProfilePath, name)
	resp, err := cdc.R().Delete(delURL)
	if err != nil {
		return err
	}
	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("delete edgex deviceProfile err: %s", string(resp.Body())) // 假定 resp.Body() 存了 msg 信息
	}
	return nil
}
