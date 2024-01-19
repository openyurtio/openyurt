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

package workloadmanager

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openyurtio/openyurt/pkg/apis/apps"
	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
)

const (
	failed  = "\u2717"
	succeed = "\u2713"
)

func TestGetWorkloadPrefix(t *testing.T) {
	tests := []struct {
		name           string
		controllerName string
		nodepoolName   string
		expect         string
	}{
		{
			"true",
			"a",
			"b",
			"a-b-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			get := getWorkloadPrefix(tt.controllerName, tt.nodepoolName)
			t.Logf("expect: %v, get: %v", tt.expect, get)
			if !reflect.DeepEqual(get, tt.expect) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
			}
			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}

func TestGetLabelSelectorFromYurtAppSet(t *testing.T) {
	tests := []struct {
		name    string
		yas     *v1beta1.YurtAppSet
		expect  *metav1.LabelSelector
		wantErr bool
	}{
		{
			"normal",
			&v1beta1.YurtAppSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-yas",
				},
			},
			&metav1.LabelSelector{
				MatchLabels: map[string]string{
					apps.YurtAppSetOwnerLabelKey: "test-yas",
				},
			},
			false,
		},
		{
			"yas is nil",
			nil,
			nil,
			true,
		},
		{
			"yas'name is empty",
			&v1beta1.YurtAppSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "",
				},
			},
			nil,
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selector, err := NewLabelSelectorForYurtAppSet(tt.yas)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetLabelSelectorFromYurtAppSet() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(selector, tt.expect) {
				t.Errorf("GetLabelSelectorFromYurtAppSet() got = %v, want %v", selector, tt.expect)
			}
		})
	}

}

func TestCreateNodeSelectorByNodepoolName(t *testing.T) {
	tests := []struct {
		name     string
		nodepool string
		expect   map[string]string
	}{
		{
			"normal",
			"a",
			map[string]string{
				projectinfo.GetNodePoolLabel(): "a",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			get := CreateNodeSelectorByNodepoolName(tt.nodepool)
			t.Logf("expect: %v, get: %v", tt.expect, get)
			if !reflect.DeepEqual(get, tt.expect) {
				t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
			}
			t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
		})
	}
}

func TestGetNodePoolsFromYurtAppSet(t *testing.T) {
	type args struct {
		cli client.Client
		yas *v1beta1.YurtAppSet
	}

	scheme := runtime.NewScheme()
	assert.Nil(t, v1beta1.AddToScheme(scheme))

	tests := []struct {
		name    string
		args    args
		wantNps []string
		wantErr bool
	}{
		{
			name: "TestGetNodePoolsFromYurtAppSetNilPools",
			args: args{
				cli: fake.NewClientBuilder().WithScheme(scheme).Build(),
				yas: &v1beta1.YurtAppSet{
					Spec: v1beta1.YurtAppSetSpec{
						Pools: nil,
					},
				},
			},
			wantNps: []string{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNps, err := GetNodePoolsFromYurtAppSet(tt.args.cli, tt.args.yas)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetNodePoolsFromYurtAppSet() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotNps.List(), tt.wantNps) {
				t.Errorf("GetNodePoolsFromYurtAppSet() gotNps = %v, want %v", gotNps.List(), tt.wantNps)
			}
		})
	}
}

// TestIsNodePoolRelated 测试isNodePoolRelated函数
func TestIsNodePoolRelated(t *testing.T) {
	// 测试用例1: pools为空，npSelector不为空，匹配成功
	nodePool := &v1beta1.NodePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "nodepool1",
			Labels: map[string]string{"label": "value"},
		},
	}
	pools := []string{}
	npSelector := &metav1.LabelSelector{
		MatchLabels: map[string]string{"label": "value"},
	}
	assert.True(t, isNodePoolRelated(nodePool, pools, npSelector))

	// 测试用例2: pools不为空，包含nodePool的名字，匹配成功
	pools = []string{"nodepool1"}
	npSelector = nil
	assert.True(t, isNodePoolRelated(nodePool, pools, npSelector))

	// 测试用例3: pools不为空，不包含nodePool的名字，匹配失败
	pools = []string{"nodepool2"}
	npSelector = nil
	assert.False(t, isNodePoolRelated(nodePool, pools, npSelector))

	// 测试用例4: pools为空，npSelector为空，匹配失败
	pools = []string{}
	npSelector = nil
	assert.False(t, isNodePoolRelated(nodePool, pools, npSelector))

	// 测试用例5: pools不为空，npSelector不为空，匹配失败
	pools = []string{"nodepool2"}
	npSelector = &metav1.LabelSelector{
		MatchLabels: map[string]string{"label": "value2"},
	}
	assert.False(t, isNodePoolRelated(nodePool, pools, npSelector))
}

// TestCombineLabels 测试CombineLabels函数，测试组合两个map的情况
func TestCombineMaps(t *testing.T) {
	// 测试case 1: label1为空，期望返回label2
	var label1 map[string]string
	label2 := map[string]string{"key1": "value1", "key2": "value2"}
	label1 = CombineMaps(label1, label2)
	expectedLabel := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}
	if !reflect.DeepEqual(label1, expectedLabel) {
		t.Errorf("CombineLabels() label1 = %v, want %v", label1, label2)
	}

	// 测试case 2: label2为空，期望返回label1
	label1 = map[string]string{"key1": "value1", "key2": "value2"}
	label2 = make(map[string]string)
	expectedLabel = map[string]string{
		"key1": "value1",
		"key2": "value2",
	}
	label1 = CombineMaps(label1, label2)
	if !reflect.DeepEqual(label1, expectedLabel) {
		t.Errorf("CombineLabels() label1 = %v, want %v", label1, label2)
	}

	// 测试case 3: label1和label2都不为空，期望返回合并后的map
	label1 = map[string]string{"key1": "value1", "key2": "value2"}
	label2 = map[string]string{"key3": "value3", "key4": "value4"}
	label1 = CombineMaps(label1, label2)
	expectedLabel = map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
		"key4": "value4",
	}
	if !reflect.DeepEqual(label1, expectedLabel) {
		t.Errorf("CombineLabels() label1 = %v, want %v", label1, label2)
	}

	// 测试case 4: label1和label2都不为空，label1和label2都有相同的key，期望返回合并后的map
	label1 = map[string]string{"key1": "value1", "key2": "value2"}
	label2 = map[string]string{"key1": "value3", "key4": "value4"}
	label1 = CombineMaps(label1, label2)
	expectedLabel = map[string]string{
		"key1": "value3",
		"key2": "value2",
		"key4": "value4",
	}
	if !reflect.DeepEqual(label1, expectedLabel) {
		t.Errorf("CombineLabels() label1 = %v, want %v", label1, label2)
	}

	// 测试case 5: label2为nil，期望返回label2
	label1 = nil
	label2 = map[string]string{"key1": "value1", "key2": "value2"}
	expectedLabel = map[string]string{
		"key1": "value1",
		"key2": "value2",
	}
	label1 = CombineMaps(label1, label2)
	if !reflect.DeepEqual(label1, expectedLabel) {
		t.Errorf("CombineLabels() label1 = %v, want %v", label1, label2)
	}
}

// TestStringsContain 测试字符串切片是否包含特定字符串
func TestStringsContain(t *testing.T) {
	// T0: 切片为空，目标字符串不为空
	strs := []string{}
	str := "test"
	assert.False(t, StringsContain(strs, str), "T0")

	// T1: 切片为空，目标字符串为空
	str = ""
	assert.False(t, StringsContain(strs, str), "T1")

	// T2: 切片不为空，目标字符串在切片内
	strs = []string{"test", "hello"}
	str = "test"
	assert.True(t, StringsContain(strs, str), "T2")

	// T3: 切片不为空，目标字符串不在切片内
	str = "world"
	assert.False(t, StringsContain(strs, str), "T3")
}

func TestGetWorkloadRefNodePool(t *testing.T) {
	type args struct {
		workload metav1.Object
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "TestGetWorkloadRefNodePool_WithAnnotation",
			args: args{
				workload: &metav1.ObjectMeta{
					Labels: map[string]string{
						apps.PoolNameLabelKey: "test-nodepool",
					},
				},
			},
			want: "test-nodepool",
		},
		{
			name: "TestGetWorkloadRefNodePool_WithoutAnnotation",
			args: args{
				workload: &metav1.ObjectMeta{},
			},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetWorkloadRefNodePool(tt.args.workload)
			if got != tt.want {
				t.Errorf("GetWorkloadRefNodePool() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetWorkloadHash(t *testing.T) {
	type args struct {
		workload metav1.Object
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "TestGetWorkloadHash_WithAnnotation",
			args: args{
				workload: &metav1.ObjectMeta{
					Labels: map[string]string{
						apps.ControllerRevisionHashLabelKey: "test-hash",
					},
				},
			},
			want: "test-hash",
		},
		{
			name: "TestGetWorkloadHash_WithoutAnnotation",
			args: args{
				workload: &metav1.ObjectMeta{},
			},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetWorkloadHash(tt.args.workload)
			if got != tt.want {
				t.Errorf("GetWorkloadHash() got = %v, want %v", got, tt.want)
			}
		})
	}
}
