/*
Copyright 2024 The OpenYurt Authors.

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

package multiplexer

import (
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/watch"

	yurtutil "github.com/openyurtio/openyurt/pkg/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
)

type filterWatch struct {
	source watch.Interface
	filter filter.ObjectFilter
	result chan watch.Event
	done   chan struct{}
}

func (f *filterWatch) Stop() {
	select {
	case <-f.done:
	default:
		close(f.done)
		f.source.Stop()
	}
}

func newFilterWatch(source watch.Interface, filter filter.ObjectFilter) watch.Interface {
	if filter == nil {
		return source
	}

	fw := &filterWatch{
		source: source,
		filter: filter,
		result: make(chan watch.Event),
		done:   make(chan struct{}),
	}

	go fw.receive()

	return fw
}

func (f *filterWatch) ResultChan() <-chan watch.Event {
	return f.result
}

func (f *filterWatch) receive() {
	defer utilruntime.HandleCrash()
	defer close(f.result)
	defer f.Stop()

	for result := range f.source.ResultChan() {
		watchType := result.Type
		newObj := result.Object
		if co, ok := newObj.(runtime.CacheableObject); ok {
			newObj = co.GetObject()
		}

		if !(result.Type == watch.Bookmark || result.Type == watch.Error) {
			if newObj = f.filter.Filter(newObj); yurtutil.IsNil(newObj) {
				watchType = watch.Deleted
				newObj = result.Object
			}
		}

		select {
		case <-f.done:
			return
		case f.result <- watch.Event{
			Type:   watchType,
			Object: newObj,
		}:
		}
	}
}
