/*
Copyright 2021 The OpenYurt Authors.

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

package phases

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurtadm/cmd/join/joindata"
	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
	"github.com/openyurtio/openyurt/pkg/yurtadm/util/initsystem"
)

// RunPostCheck executes the node health check process.
func RunPostCheck(data joindata.YurtJoinData) error {
	klog.V(1).Infof("check kubelet status.")
	if err := checkKubeletStatus(); err != nil {
		return err
	}
	klog.V(1).Infof("kubelet service is active")

	klog.V(1).Infof("waiting hub agent ready.")
	if err := checkYurthubHealthz(data); err != nil {
		return err
	}
	klog.V(1).Infof("hub agent is ready")

	return nil
}

// checkKubeletStatus check if kubelet is healthy.
func checkKubeletStatus() error {
	initSystem, err := initsystem.GetInitSystem()
	if err != nil {
		return err
	}
	if ok := initSystem.ServiceIsActive("kubelet"); !ok {
		return fmt.Errorf("kubelet is not active. ")
	}
	return nil
}

// checkYurthubHealthz check if YurtHub is healthy.
func checkYurthubHealthz(joinData joindata.YurtJoinData) error {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s%s", fmt.Sprintf("%s:10267", joinData.YurtHubServer()), constants.ServerHealthzURLPath), nil)
	if err != nil {
		return err
	}
	client := &http.Client{}
	return wait.PollImmediate(time.Second*5, 300*time.Second, func() (bool, error) {
		resp, err := client.Do(req)
		if err != nil {
			return false, nil
		}
		ok, err := io.ReadAll(resp.Body)
		if err != nil {
			return false, nil
		}
		return string(ok) == "OK", nil
	})
}
