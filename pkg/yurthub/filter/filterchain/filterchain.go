package filterchain

import (
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"

	yurtutil "github.com/openyurtio/openyurt/pkg/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
)

type FilterChain []filter.ObjectFilter

func (chain FilterChain) Name() string {
	var names []string
	for i := range chain {
		names = append(names, chain[i].Name())
	}
	return strings.Join(names, ",")
}

func (chain FilterChain) SupportedResourceAndVerbs() map[string]sets.Set[string] {
	// do nothing
	return map[string]sets.Set[string]{}
}

func (chain FilterChain) Filter(obj runtime.Object, stopCh <-chan struct{}) runtime.Object {
	for i := range chain {
		obj = chain[i].Filter(obj, stopCh)
		if yurtutil.IsNil(obj) {
			break
		}
	}

	return obj
}
