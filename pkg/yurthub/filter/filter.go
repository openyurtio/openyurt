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

package filter

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/watch"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

type Factory func() (Runner, error)

type Filters struct {
	sync.Mutex
	names           []string
	registry        map[string]Factory
	disabledFilters sets.String
}

func NewFilters(disabledFilters []string) *Filters {
	return &Filters{
		names:           make([]string, 0),
		registry:        make(map[string]Factory),
		disabledFilters: sets.NewString(disabledFilters...),
	}
}

func (fs *Filters) NewFromFilters(initializer FilterInitializer) ([]Runner, error) {
	var runners = make([]Runner, 0)
	for _, name := range fs.names {
		if fs.Enabled(name) {
			factory, found := fs.registry[name]
			if !found {
				return nil, fmt.Errorf("Filter %s has not registered", name)
			}

			ins, err := factory()
			if err != nil {
				klog.Errorf("new filter %s failed, %v", name, err)
				return nil, err
			}

			if err = initializer.Initialize(ins); err != nil {
				klog.Errorf("Filter %s initialize failed, %v", name, err)
				return nil, err
			}
			klog.V(2).Infof("Filter %s initialize successfully", name)
			runners = append(runners, ins)
		} else {
			klog.V(2).Infof("Filter %s is disabled", name)
		}
	}

	return runners, nil
}

func (fs *Filters) Register(name string, fn Factory) {
	fs.Lock()
	defer fs.Unlock()

	_, found := fs.registry[name]
	if found {
		klog.Warningf("Filter %q has already registered", name)
		return
	}

	klog.V(2).Infof("Filter %s registered successfully", name)
	fs.registry[name] = fn
	fs.names = append(fs.names, name)
}

func (fs *Filters) Enabled(name string) bool {
	if fs.disabledFilters.Len() == 1 && fs.disabledFilters.Has("*") {
		return false
	}

	return !fs.disabledFilters.Has(name)
}

type FilterInitializers []FilterInitializer

func (fis FilterInitializers) Initialize(ins Runner) error {
	for _, fi := range fis {
		if err := fi.Initialize(ins); err != nil {
			return err
		}
	}

	return nil
}

type filterReadCloser struct {
	rc          io.ReadCloser
	filterCache *bytes.Buffer
	watchDataCh chan *bytes.Buffer
	serializer  *serializer.Serializer
	handler     ObjectHandler
	isWatch     bool
	ownerName   string
	stopCh      <-chan struct{}
}

// NewFilterReadCloser create an filterReadCloser object
func NewFilterReadCloser(
	req *http.Request,
	sm *serializer.SerializerManager,
	rc io.ReadCloser,
	handler ObjectHandler,
	ownerName string,
	stopCh <-chan struct{}) (int, io.ReadCloser, error) {
	ctx := req.Context()
	info, _ := apirequest.RequestInfoFrom(ctx)
	respContentType, _ := util.RespContentTypeFrom(ctx)
	s := CreateSerializer(respContentType, info, sm)
	if s == nil {
		klog.Errorf("skip filter, failed to create serializer in %s", ownerName)
		return 0, rc, nil
	}

	frc := &filterReadCloser{
		rc:          rc,
		watchDataCh: make(chan *bytes.Buffer),
		filterCache: new(bytes.Buffer),
		serializer:  s,
		handler:     handler,
		isWatch:     info.Verb == "watch",
		ownerName:   ownerName,
		stopCh:      stopCh,
	}

	if frc.isWatch {
		go func(req *http.Request, rc io.ReadCloser, ch chan *bytes.Buffer) {
			err := frc.StreamResponseFilter(rc, ch)
			if err != nil && err != io.EOF && !errors.Is(err, context.Canceled) {
				klog.Errorf("filter(%s) watch response ended with error, %v", frc.ownerName, err)
			}
		}(req, rc, frc.watchDataCh)
		return 0, frc, nil
	} else {
		var err error
		frc.filterCache, err = frc.ObjectResponseFilter(rc)
		return frc.filterCache.Len(), frc, err
	}
}

// Read get data into p and write into pipe
func (frc *filterReadCloser) Read(p []byte) (int, error) {
	var ok bool
	if frc.isWatch {
		if frc.filterCache.Len() != 0 {
			return frc.filterCache.Read(p)
		} else {
			frc.filterCache.Reset()
		}

		select {
		case frc.filterCache, ok = <-frc.watchDataCh:
			if !ok {
				return 0, io.EOF
			}
			return frc.filterCache.Read(p)
		}
	} else {
		return frc.filterCache.Read(p)
	}
}

// Close will close readers
func (frc *filterReadCloser) Close() error {
	if frc.filterCache != nil {
		frc.filterCache.Reset()
	}
	return frc.rc.Close()
}

func (frc *filterReadCloser) ObjectResponseFilter(rc io.ReadCloser) (*bytes.Buffer, error) {
	var buf bytes.Buffer
	_, err := buf.ReadFrom(rc)
	if err != nil {
		return &buf, err
	}
	obj, err := frc.serializer.Decode(buf.Bytes())
	if err != nil || obj == nil {
		klog.Errorf("skip filter, failed to decode response in HandleObjectResponse of %s %v", frc.ownerName, err)
		return &buf, nil
	}

	filteredObj, isNil := frc.handler.RuntimeObjectFilter(obj)
	if isNil {
		return &buf, nil
	}

	newData, err := frc.serializer.Encode(filteredObj)
	return bytes.NewBuffer(newData), err
}

func (frc *filterReadCloser) StreamResponseFilter(rc io.ReadCloser, ch chan *bytes.Buffer) error {
	defer close(ch)

	d, err := frc.serializer.WatchDecoder(rc)
	if err != nil {
		klog.Errorf("failed to get watch decoder in StreamResponseFilter of %s, %v", frc.ownerName, err)
		return err
	}

	for {
		watchType, obj, err := d.Decode()
		if err != nil {
			return err
		}

		newObj, isNil := frc.handler.RuntimeObjectFilter(obj)
		if isNil {
			continue
		}

		wEvent := watch.Event{
			Type:   watchType,
			Object: newObj,
		}

		buf := &bytes.Buffer{}
		_, err = frc.serializer.WatchEncode(buf, &wEvent)
		if err != nil {
			klog.Errorf("failed to encode resource in StreamResponseFilter of %s, %v", frc.ownerName, err)
			return err
		}
		ch <- buf
	}
}

func CreateSerializer(respContentType string, info *apirequest.RequestInfo, sm *serializer.SerializerManager) *serializer.Serializer {
	if respContentType == "" || info == nil || info.APIVersion == "" || info.Resource == "" {
		klog.Infof("CreateSerializer failed , info is :%+v", info)
		return nil
	}
	return sm.CreateSerializer(respContentType, info.APIGroup, info.APIVersion, info.Resource)
}
