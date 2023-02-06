/*
Copyright 2022 The OpenYurt Authors.

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

package cert

import (
	"net"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

const (
	failed  = "\u2717"
	succeed = "\u2713"
)

func TestGetURLFromSVC(t *testing.T) {
	tests := []struct {
		name   string
		svc    *corev1.Service
		expect string
		err    error
	}{
		{
			name: "normal service",
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					ClusterIP: "xxxx",
					Ports: []corev1.ServicePort{
						{
							Port: 644,
						},
					},
				},
			},
			expect: "https://xxxx:644",
			err:    nil,
		},
		{
			name: "service port missing",
			svc: &corev1.Service{
				Spec: corev1.ServiceSpec{
					ClusterIP: "xxxx",
				},
			},
			expect: "",
			err:    errors.New("Service port list cannot be empty"),
		},
	}

	for _, tt := range tests {
		st := tt
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", st.name)
			{
				url, err := GetURLFromSVC(st.svc)
				if url != st.expect || err != nil && errors.Is(err, st.err) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, st.expect, url)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, st.expect, url)

			}
		}
		t.Run(st.name, tf)

	}
}

func TestGetAPIServerSVCURL(t *testing.T) {
	emptyClient := fake.NewSimpleClientset()

	_, err := getAPIServerSVCURL(emptyClient)
	if !kerrors.IsNotFound(err) {
		t.Fatalf("\t%s\texpect not found err, but get %v", failed, err)
	}

	normalClient := fake.NewSimpleClientset(&corev1.Service{
		ObjectMeta: v1.ObjectMeta{
			Namespace: PoolcoordinatorNS,
			Name:      PoolcoordinatorAPIServerSVC,
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "xxxx",
			Ports: []corev1.ServicePort{
				{
					Port: 644,
				},
			},
		},
	})
	url, err := getAPIServerSVCURL(normalClient)
	assert.Equal(t, nil, err)
	assert.Equal(t, "https://xxxx:644", url)
}

func TestSearchIPs(t *testing.T) {

	// empty ip list
	ipList := []net.IP{}
	assert.False(t, searchIP(ipList, net.ParseIP("10.0.0.1")))

	// ip list with multiple ips
	ipList = []net.IP{
		net.ParseIP("10.0.0.1"),
		net.ParseIP("10.0.0.2"),
	}
	assert.True(t, searchIP(ipList, net.ParseIP("10.0.0.1")))
	assert.True(t, searchIP(ipList, net.ParseIP("10.0.0.2")))
	assert.False(t, searchIP(ipList, net.ParseIP("10.0.0.3")))

	// search one exiting ip
	ips := []net.IP{
		net.ParseIP("10.0.0.1"),
	}
	assert.True(t, searchAllIP(ipList, ips))

	// search multiple existing ips
	ips = []net.IP{
		net.ParseIP("10.0.0.1"),
		net.ParseIP("10.0.0.2"),
	}
	assert.True(t, searchAllIP(ipList, ips))

	// search multiple existing ips with one missing ip
	ips = []net.IP{
		net.ParseIP("10.0.0.3"),
	}
	assert.False(t, searchAllIP(ipList, ips))

	// search multiple existing ips with multiple missing ips
	ips = []net.IP{
		net.ParseIP("10.0.0.1"),
		net.ParseIP("10.0.0.4"),
		net.ParseIP("10.0.0.3"),
		net.ParseIP("10.0.0.2"),
	}
	assert.False(t, searchAllIP(ipList, ips))
}
