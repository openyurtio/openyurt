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

package clients

import (
	"context"

	"github.com/edgexfoundry/go-mod-core-contracts/v3/dtos"

	iotv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/iot/v1alpha1"
)

// CreateOptions defines additional options when creating an object
// Additional general field definitions can be added
type CreateOptions struct{}

// DeleteOptions defines additional options when deleting an object
// Additional general field definitions can be added
type DeleteOptions struct{}

// UpdateOptions defines additional options when updating an object
// Additional general field definitions can be added
type UpdateOptions struct{}

// GetOptions defines additional options when getting an object
// Additional general field definitions can be added
type GetOptions struct {
	// Namespace represents the namespace to list for, or empty for
	// non-namespaced objects, or to list across all namespaces.
	Namespace string
}

// ListOptions defines additional options when listing an object
type ListOptions struct {
	// A selector to restrict the list of returned objects by their labels.
	// Defaults to everything.
	// +optional
	LabelSelector map[string]string
	// A selector to restrict the list of returned objects by their fields.
	// Defaults to everything.
	// +optional
	FieldSelector map[string]string
	// Namespace represents the namespace to list for, or empty for
	// non-namespaced objects, or to list across all namespaces.
	Namespace string
}

// DeviceInterface defines the interfaces which used to create, delete, update, get and list Device objects on edge-side platform
type DeviceInterface interface {
	DevicePropertyInterface
	Create(ctx context.Context, device *iotv1alpha1.Device, options CreateOptions) (*iotv1alpha1.Device, error)
	Delete(ctx context.Context, name string, options DeleteOptions) error
	Update(ctx context.Context, device *iotv1alpha1.Device, options UpdateOptions) (*iotv1alpha1.Device, error)
	Get(ctx context.Context, name string, options GetOptions) (*iotv1alpha1.Device, error)
	List(ctx context.Context, options ListOptions) ([]iotv1alpha1.Device, error)

	Convert(ctx context.Context, systemEvent dtos.SystemEvent, options GetOptions) (*iotv1alpha1.Device, error)
}

// DevicePropertyInterface defines the interfaces which used to get, list and set the actual status value of the device properties
type DevicePropertyInterface interface {
	GetPropertyState(ctx context.Context, propertyName string, device *iotv1alpha1.Device, options GetOptions) (*iotv1alpha1.ActualPropertyState, error)
	UpdatePropertyState(ctx context.Context, propertyName string, device *iotv1alpha1.Device, options UpdateOptions) error
	ListPropertiesState(ctx context.Context, device *iotv1alpha1.Device, options ListOptions) (map[string]iotv1alpha1.DesiredPropertyState, map[string]iotv1alpha1.ActualPropertyState, error)
}

// DeviceServiceInterface defines the interfaces which used to create, delete, update, get and list DeviceService objects on edge-side platform
type DeviceServiceInterface interface {
	Create(ctx context.Context, deviceService *iotv1alpha1.DeviceService, options CreateOptions) (*iotv1alpha1.DeviceService, error)
	Delete(ctx context.Context, name string, options DeleteOptions) error
	Update(ctx context.Context, deviceService *iotv1alpha1.DeviceService, options UpdateOptions) (*iotv1alpha1.DeviceService, error)
	Get(ctx context.Context, name string, options GetOptions) (*iotv1alpha1.DeviceService, error)
	List(ctx context.Context, options ListOptions) ([]iotv1alpha1.DeviceService, error)

	Convert(ctx context.Context, systemEvent dtos.SystemEvent, opts GetOptions) (*iotv1alpha1.DeviceService, error)
}

// DeviceProfileInterface defines the interfaces which used to create, delete, update, get and list DeviceProfile objects on edge-side platform
type DeviceProfileInterface interface {
	Create(ctx context.Context, deviceProfile *iotv1alpha1.DeviceProfile, options CreateOptions) (*iotv1alpha1.DeviceProfile, error)
	Delete(ctx context.Context, name string, options DeleteOptions) error
	Update(ctx context.Context, deviceProfile *iotv1alpha1.DeviceProfile, options UpdateOptions) (*iotv1alpha1.DeviceProfile, error)
	Get(ctx context.Context, name string, options GetOptions) (*iotv1alpha1.DeviceProfile, error)
	List(ctx context.Context, options ListOptions) ([]iotv1alpha1.DeviceProfile, error)

	Convert(ctx context.Context, systemEvent dtos.SystemEvent, options GetOptions) (*iotv1alpha1.DeviceProfile, error)
}

// IoTDock defines the interfaces which used to create deviceclient, deviceprofileclient, deviceserviceclient
type IoTDock interface {
	CreateDeviceClient() (DeviceInterface, error)
	CreateDeviceProfileClient() (DeviceProfileInterface, error)
	CreateDeviceServiceClient() (DeviceServiceInterface, error)
}
