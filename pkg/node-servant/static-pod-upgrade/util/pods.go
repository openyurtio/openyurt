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

package util

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

const (
	YurtHubAddress = "http://127.0.0.1:10267"
	YurtHubAPIPath = "/pods"
)

func GetPodFromYurtHub(namespace, name string) (*v1.Pod, error) {
	podList, err := GetPodsFromYurtHub(YurtHubAddress + YurtHubAPIPath)
	if err != nil {
		return nil, err
	}

	for i, pod := range podList.Items {
		if pod.Namespace == namespace && pod.Name == name {
			return &podList.Items[i], nil
		}
	}

	return nil, fmt.Errorf("could not find pod %s/%s", namespace, name)
}

func GetPodsFromYurtHub(url string) (*v1.PodList, error) {
	data, err := getPodsDataFromYurtHub(url)
	if err != nil {
		return nil, err
	}

	podList, err := decodePods(data)
	if err != nil {
		return nil, err
	}

	return podList, nil
}

func getPodsDataFromYurtHub(url string) ([]byte, error) {
	// avoid accessing conflict
	rand.Seed(time.Now().UnixNano())
	time.Sleep(time.Duration(rand.Intn(1000)) * time.Millisecond)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("could not access yurthub pods API, returned status: %v", resp.Status)
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func decodePods(data []byte) (*v1.PodList, error) {
	codecFactory := serializer.NewCodecFactory(runtime.NewScheme())
	codec := codecFactory.LegacyCodec(schema.GroupVersion{Group: v1.GroupName, Version: "v1"})

	podList := new(v1.PodList)
	if _, _, err := codec.Decode(data, nil, podList); err != nil {
		return nil, fmt.Errorf("could not decode pod list: %s", err)
	}
	return podList, nil
}
