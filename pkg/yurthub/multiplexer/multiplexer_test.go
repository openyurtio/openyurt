package multiplexer

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/meta"
	"github.com/openyurtio/openyurt/pkg/yurthub/multiplexer/storage"
)

func TestMultiplexerManager_UpdateLeaderHubConfiguration(t *testing.T) {
	client := fake.NewSimpleClientset()
	informerFactory := informers.NewSharedInformerFactory(client, 24*time.Hour)

	tmpDir, err := os.MkdirTemp("", "test")
	if err != nil {
		t.Fatalf("failed to make temp dir, %v", err)
	}
	defer os.RemoveAll(tmpDir)

	restMapperManager, _ := meta.NewRESTMapperManager(tmpDir)

	cfg := &config.YurtHubConfiguration{
		SharedFactory:      informerFactory,
		RESTMapperManager:  restMapperManager,
		NodePoolName:       "test-pool",
		NodeName:           "node1",
		PoolScopeResources: []schema.GroupVersionResource{},
	}

	sp := storage.NewDummyStorageManager(mockCacheMap())
	mgr := NewRequestMultiplexerManager(cfg, sp, nil)

	// Inject a fake filterStore into the manager directly to spy on Destroy
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	gvrStr := gvr.String()

	destroyCalled := false
	fakeStore, _ := mgr.filterStoreManager.genericStore(&endpointSliceGVR) // use valid gvr for generic store
	fakeStore.DestroyFunc = func() {
		destroyCalled = true
	}

	mgr.filterStoreManager.filterStores[gvrStr] = &filterStore{
		store: fakeStore,
		gvr:   &gvr,
	}

	// 1. Simulate adding GVR to pool-scoped metadata
	addCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "leader-hub-test-pool",
		},
		Data: map[string]string{
			PoolScopeMetadataKey: "apps/v1/deployments",
		},
	}
	mgr.updateLeaderHubConfiguration(addCM)

	assert.Equal(t, sets.New[string]("apps/v1/deployments"), mgr.poolScopeMetadata)
	assert.False(t, destroyCalled)

	// 2. Simulate removing the GVR
	removeCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "leader-hub-test-pool",
		},
		Data: map[string]string{
			PoolScopeMetadataKey: "",
		},
	}
	mgr.updateLeaderHubConfiguration(removeCM)

	// 3. Assert Destroy was called
	assert.True(t, destroyCalled, "Expected filterStore.Destroy() to be called")
	assert.Equal(t, sets.New[string](), mgr.poolScopeMetadata)
	
	// Ensure it's removed from map
	_, exists := mgr.filterStoreManager.filterStores[gvrStr]
	assert.False(t, exists, "Expected filterStore to be removed from map")
}
