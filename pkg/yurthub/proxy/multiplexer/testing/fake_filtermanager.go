package testing

import (
	"net/http"

	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
)

type EmptyFilterManager struct {
}

func (fm *EmptyFilterManager) FindResponseFilter(req *http.Request) (filter.ResponseFilter, bool) {
	return nil, false
}

func (fm *EmptyFilterManager) FindObjectFilters(req *http.Request) filter.ObjectFilter {
	return nil
}

type FakeEndpointSliceFilter struct {
	NodeName string
}

func (fm *FakeEndpointSliceFilter) FindResponseFilter(req *http.Request) (filter.ResponseFilter, bool) {
	return nil, false
}

func (fm *FakeEndpointSliceFilter) FindObjectFilters(req *http.Request) filter.ObjectFilter {
	return &IgnoreEndpointslicesWithNodeName{
		fm.NodeName,
	}
}
