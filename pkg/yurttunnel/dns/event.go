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

package dns

type EventType string

const (
	NodeAdd         EventType = "NODE_ADD"
	NodeUpdate      EventType = "NODE_UPDATE"
	NodeDelete      EventType = "NODE_DELETE"
	ServiceAdd      EventType = "SERVICE_ADD"
	ServiceUpdate   EventType = "SERVICE_UPDATE"
	ServiceDelete   EventType = "SERVICE_DELETE"
	ConfigMapAdd    EventType = "CONFIGMAP_ADD"
	ConfigMapUpdate EventType = "CONFIGMAP_UPDATE"
	ConfigMapDelete EventType = "CONFIGMAP_DELETE"
)

type Event struct {
	Obj  interface{}
	Type EventType
}
