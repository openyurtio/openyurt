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

func (fs *Filters) NewFromFilters(initializer FilterInitializer) (map[string]Runner, error) {
	var filterMapping = make(map[string]Runner)
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
			filterMapping[name] = ins
		}
	}

	if len(filterMapping) == 0 {
		return nil, nil
	}

	return filterMapping, nil
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
	req        *http.Request
	rc         io.ReadCloser
	data       *bytes.Buffer
	ch         chan watch.Event
	handler    Handler
	isWatch    bool
	serializer *serializer.Serializer
	ownerName  string
	stopCh     <-chan struct{}
}

// NewFilterReadCloser create an filterReadCloser object
func NewFilterReadCloser(
	req *http.Request,
	rc io.ReadCloser,
	handler Handler,
	serializer *serializer.Serializer,
	ownerName string,
	stopCh <-chan struct{}) (int, io.ReadCloser, error) {

	ctx := req.Context()
	info, _ := apirequest.RequestInfoFrom(ctx)
	dr := &filterReadCloser{
		req:        req,
		rc:         rc,
		ch:         make(chan watch.Event),
		data:       new(bytes.Buffer),
		handler:    handler,
		isWatch:    info.Verb == "watch",
		serializer: serializer,
		ownerName:  ownerName,
		stopCh:     stopCh,
	}

	if dr.isWatch {
		go func(req *http.Request, rc io.ReadCloser, ch chan watch.Event) {
			err := handler.StreamResponseFilter(rc, ch)
			if err != nil && err != io.EOF && !errors.Is(err, context.Canceled) {
				klog.Errorf("filter(%s) watch response ended with error, %v", dr.ownerName, err)
			}
		}(req, rc, dr.ch)
		return 0, dr, nil
	} else {
		var newData []byte
		n, err := dr.data.ReadFrom(rc)
		if err != nil {
			return int(n), dr, err
		}

		newData, err = handler.ObjectResponseFilter(dr.data.Bytes())
		dr.data = bytes.NewBuffer(newData)
		return len(newData), dr, err
	}
}

// Read read data into p and write into pipe
func (dr *filterReadCloser) Read(p []byte) (int, error) {
	if dr.isWatch {
		select {
		case watchEvent, ok := <-dr.ch:
			if !ok {
				return 0, io.EOF
			}

			buf := &bytes.Buffer{}
			n, err := dr.serializer.WatchEncode(buf, &watchEvent)
			if err != nil {
				klog.Errorf("filter(%s) failed to encode resource in Reader %v", dr.ownerName, err)
				return 0, err
			}
			copied := copy(p, buf.Bytes())
			if copied != n {
				return 0, fmt.Errorf("filter(%s) expect copy %d bytes, but only %d bytes copyied", dr.ownerName, n, copied)
			}

			return n, nil
		}
	} else {
		return dr.data.Read(p)
	}
}

// Close close readers
func (dr *filterReadCloser) Close() error {
	return dr.rc.Close()
}
