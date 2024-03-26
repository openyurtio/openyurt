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

package responsefilter

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/watch"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/klog/v2"

	yurtutil "github.com/openyurtio/openyurt/pkg/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

type filterReadCloser struct {
	rc           io.ReadCloser
	filterCache  *bytes.Buffer
	watchDataCh  chan *bytes.Buffer
	serializer   *serializer.Serializer
	objectFilter filter.ObjectFilter
	isWatch      bool
	isList       bool
	ownerName    string
	stopCh       <-chan struct{}
}

// newFilterReadCloser create an filterReadCloser object
func newFilterReadCloser(
	req *http.Request,
	sm *serializer.SerializerManager,
	rc io.ReadCloser,
	objectFilter filter.ObjectFilter,
	ownerName string,
	stopCh <-chan struct{}) (int, io.ReadCloser, error) {
	ctx := req.Context()
	info, _ := apirequest.RequestInfoFrom(ctx)
	respContentType, _ := util.RespContentTypeFrom(ctx)
	s := createSerializer(respContentType, info, sm)
	if s == nil {
		klog.Errorf("skip filter, could not create serializer in %s", ownerName)
		return 0, rc, nil
	}

	frc := &filterReadCloser{
		rc:           rc,
		watchDataCh:  make(chan *bytes.Buffer),
		filterCache:  new(bytes.Buffer),
		serializer:   s,
		objectFilter: objectFilter,
		isWatch:      info.Verb == "watch",
		isList:       info.Verb == "list",
		ownerName:    ownerName,
		stopCh:       stopCh,
	}

	if frc.isWatch {
		go func(req *http.Request, rc io.ReadCloser, ch chan *bytes.Buffer) {
			err := frc.streamResponseFilter(rc, ch)
			if err != nil && err != io.EOF && !errors.Is(err, context.Canceled) {
				klog.Errorf("filter(%s) watch response ended with error, %v", frc.ownerName, err)
			}
		}(req, rc, frc.watchDataCh)
		return 0, frc, nil
	} else {
		var err error
		frc.filterCache, err = frc.objectResponseFilter(rc)
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

func (frc *filterReadCloser) objectResponseFilter(rc io.ReadCloser) (*bytes.Buffer, error) {
	var buf bytes.Buffer
	_, err := buf.ReadFrom(rc)
	if err != nil {
		return &buf, err
	}
	obj, err := frc.serializer.Decode(buf.Bytes())
	if err != nil || obj == nil {
		klog.Errorf("skip filter, could not decode response in HandleObjectResponse of %s %v", frc.ownerName, err)
		return &buf, nil
	}

	if frc.isList {
		items, err := meta.ExtractList(obj)
		if err != nil || len(items) == 0 {
			obj = frc.objectFilter.Filter(obj, frc.stopCh)
		} else {
			list := make([]runtime.Object, 0)
			for i := range items {
				newObj := frc.objectFilter.Filter(items[i], frc.stopCh)
				if !yurtutil.IsNil(newObj) {
					list = append(list, newObj)
				}
			}
			if err = meta.SetList(obj, list); err != nil {
				klog.Warningf("filter %s doesn't work correctly, couldn't set list, %v.", frc.ownerName, err)
				return &buf, nil
			}
		}
	} else {
		obj = frc.objectFilter.Filter(obj, frc.stopCh)
	}
	if yurtutil.IsNil(obj) {
		klog.Warningf("filter %s doesn't work correctly, response is discarded completely in list request.", frc.ownerName)
		return &buf, nil
	}

	newData, err := frc.serializer.Encode(obj)
	return bytes.NewBuffer(newData), err
}

func (frc *filterReadCloser) streamResponseFilter(rc io.ReadCloser, ch chan *bytes.Buffer) error {
	defer close(ch)

	d, err := frc.serializer.WatchDecoder(rc)
	if err != nil {
		klog.Errorf("could not get watch decoder in StreamResponseFilter of %s, %v", frc.ownerName, err)
		return err
	}

	for {
		watchType, obj, err := d.Decode()
		if err != nil {
			return err
		}

		newObj := obj
		// BOOKMARK and ERROR response are unnecessary to filter
		if !(watchType == watch.Bookmark || watchType == watch.Error) {
			if newObj = frc.objectFilter.Filter(obj, frc.stopCh); yurtutil.IsNil(newObj) {
				// if an object is removed in the filter chain, it means that this object is not needed
				// to return back to clients(like kube-proxy). but in order to update the client's local cache,
				// it's a good idea to return a watch.Deleted event to clients and make clients to remove this object in local cache.
				watchType = watch.Deleted
				newObj = obj
			}
		}

		wEvent := watch.Event{
			Type:   watchType,
			Object: newObj,
		}

		buf := &bytes.Buffer{}
		_, err = frc.serializer.WatchEncode(buf, &wEvent)
		if err != nil {
			klog.Errorf("could not encode resource in StreamResponseFilter of %s, %v", frc.ownerName, err)
			return err
		}
		ch <- buf
	}
}

func createSerializer(respContentType string, info *apirequest.RequestInfo, sm *serializer.SerializerManager) *serializer.Serializer {
	if respContentType == "" || info == nil || info.APIVersion == "" || info.Resource == "" {
		klog.Infof("CreateSerializer failed , info is :%+v", info)
		return nil
	}
	return sm.CreateSerializer(respContentType, info.APIGroup, info.APIVersion, info.Resource)
}

type filterChain []filter.ObjectFilter

func createFilterChain(objFilters []filter.ObjectFilter) filter.ObjectFilter {
	chain := make(filterChain, 0)
	chain = append(chain, objFilters...)
	return chain
}

func (chain filterChain) Name() string {
	var names []string
	for i := range chain {
		names = append(names, chain[i].Name())
	}
	return strings.Join(names, ",")
}

func (chain filterChain) SupportedResourceAndVerbs() map[string]sets.String {
	// do nothing
	return map[string]sets.String{}
}

func (chain filterChain) Filter(obj runtime.Object, stopCh <-chan struct{}) runtime.Object {
	for i := range chain {
		obj = chain[i].Filter(obj, stopCh)
		if yurtutil.IsNil(obj) {
			break
		}
	}

	return obj
}

type responseFilter struct {
	objectFilter      filter.ObjectFilter
	serializerManager *serializer.SerializerManager
}

func CreateResponseFilter(objectFilters []filter.ObjectFilter, serializerManager *serializer.SerializerManager) filter.ResponseFilter {
	return &responseFilter{
		objectFilter:      createFilterChain(objectFilters),
		serializerManager: serializerManager,
	}
}

func (rf *responseFilter) Name() string {
	return rf.objectFilter.Name()
}

func (rf *responseFilter) Filter(req *http.Request, rc io.ReadCloser, stopCh <-chan struct{}) (int, io.ReadCloser, error) {
	return newFilterReadCloser(req, rf.serializerManager, rc, rf.objectFilter, rf.objectFilter.Name(), stopCh)
}
