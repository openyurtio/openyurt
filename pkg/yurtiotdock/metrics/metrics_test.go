package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	iotv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/iot/v1alpha1"
)

func TestIoTCollector(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = iotv1alpha1.AddToScheme(scheme)

	d1 := &iotv1alpha1.Device{
		ObjectMeta: metav1.ObjectMeta{Name: "device1"},
		Spec:       iotv1alpha1.DeviceSpec{NodePool: "pool1"},
		Status:     iotv1alpha1.DeviceStatus{Synced: true},
	}
	d2 := &iotv1alpha1.Device{
		ObjectMeta: metav1.ObjectMeta{Name: "device2"},
		Spec:       iotv1alpha1.DeviceSpec{NodePool: "pool1"},
		Status:     iotv1alpha1.DeviceStatus{Synced: false},
	}

	ds1 := &iotv1alpha1.DeviceService{
		ObjectMeta: metav1.ObjectMeta{Name: "ds1"},
		Spec:       iotv1alpha1.DeviceServiceSpec{NodePool: "pool1"},
	}

	dp1 := &iotv1alpha1.DeviceProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "dp1"},
		Spec:       iotv1alpha1.DeviceProfileSpec{NodePool: "pool1"},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(d1, d2, ds1, dp1).Build()

	collector := &iotCollector{client: fakeClient}

	ch := make(chan prometheus.Metric, 10)
	go func() {
		collector.Collect(ch)
		close(ch)
	}()

	var count int
	for range ch {
		count++
	}

	if count != 4 { // 2 devices (synced + unsynced) + 1 DS + 1 DP = 4 metrics
		t.Fatalf("expected 4 metrics, got %d", count)
	}
}
