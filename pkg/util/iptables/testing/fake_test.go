/*
Copyright 2015 The Kubernetes Authors.

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

package testing

import (
	"bytes"
	"reflect"
	"testing"
	"time"

	"github.com/openyurtio/openyurt/pkg/util/iptables"
)

const (
	failed  = "\u2717"
	succeed = "\u2713"
)

func TestNewFake(t *testing.T) {
	tests := []struct {
		name   string
		expect *FakeIPTables
	}{
		{
			"normal",
			&FakeIPTables{},
		},
	}

	for _, tt := range tests {
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := NewFake()

				if !reflect.DeepEqual(*tt.expect, *get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)

			}
		}
		t.Run(tt.name, tf)
	}
}

func TestNewIpv6Fake(t *testing.T) {
	tests := []struct {
		name   string
		expect *FakeIPTables
	}{
		{
			"normal",
			&FakeIPTables{ipv6: true},
		},
	}

	for _, tt := range tests {
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := NewIpv6Fake()

				if !reflect.DeepEqual(*tt.expect, *get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)

			}
		}
		t.Run(tt.name, tf)
	}
}

func TestSetHasRandomFully(t *testing.T) {
	f := FakeIPTables{}

	tests := []struct {
		name   string
		can    bool
		expect *FakeIPTables
	}{
		{
			"normal",
			true,
			&FakeIPTables{
				hasRandomFully: true,
				ipv6:           false,
			},
		},
	}

	for _, tt := range tests {
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				var get = f.SetHasRandomFully(true)
				if !reflect.DeepEqual(*tt.expect, *get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)

			}
		}
		t.Run(tt.name, tf)
	}
}

func TestEnsureChain(t *testing.T) {
	f := FakeIPTables{}

	tests := []struct {
		name   string
		table  iptables.Table
		chain  iptables.Chain
		expect bool
	}{
		{
			"normal",
			"",
			"",
			true,
		},
	}

	for _, tt := range tests {
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get, _ := f.EnsureChain(tt.table, tt.chain)
				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)

			}
		}
		t.Run(tt.name, tf)
	}
}

func TestFlushChain(t *testing.T) {
	f := FakeIPTables{}

	tests := []struct {
		name   string
		table  iptables.Table
		chain  iptables.Chain
		expect error
	}{
		{
			"normal",
			"",
			"",
			nil,
		},
	}

	for _, tt := range tests {
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := f.FlushChain(tt.table, tt.chain)
				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)

			}
		}
		t.Run(tt.name, tf)
	}
}

func TestDeleteChain(t *testing.T) {
	f := FakeIPTables{}

	tests := []struct {
		name   string
		table  iptables.Table
		chain  iptables.Chain
		expect error
	}{
		{
			"normal",
			"",
			"",
			nil,
		},
	}

	for _, tt := range tests {
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := f.DeleteChain(tt.table, tt.chain)
				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)

			}
		}
		t.Run(tt.name, tf)
	}
}

func TestEnsureRule(t *testing.T) {
	f := FakeIPTables{}

	tests := []struct {
		name     string
		position iptables.RulePosition
		table    iptables.Table
		chain    iptables.Chain
		expect   error
	}{
		{
			"normal",
			"",
			"",
			"",
			nil,
		},
	}

	for _, tt := range tests {
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				_, get := f.EnsureRule(tt.position, tt.table, tt.chain)
				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)

			}
		}
		t.Run(tt.name, tf)
	}
}

func TestDeleteRule(t *testing.T) {
	f := FakeIPTables{}

	tests := []struct {
		name   string
		table  iptables.Table
		chain  iptables.Chain
		expect error
	}{
		{
			"normal",
			"",
			"",
			nil,
		},
	}

	for _, tt := range tests {
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := f.DeleteRule(tt.table, tt.chain)
				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)

			}
		}
		t.Run(tt.name, tf)
	}
}

func TestIsIpv6(t *testing.T) {
	f := FakeIPTables{
		ipv6: true,
	}

	tests := []struct {
		name   string
		expect bool
	}{
		{
			"normal",
			true,
		},
	}

	for _, tt := range tests {
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := f.IsIpv6()
				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)

			}
		}
		t.Run(tt.name, tf)
	}
}

func TestSave(t *testing.T) {
	f := FakeIPTables{}

	tests := []struct {
		name   string
		table  iptables.Table
		expect error
	}{
		{
			"normal",
			"",
			nil,
		},
	}

	for _, tt := range tests {
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				_, get := f.Save(tt.table)
				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)

			}
		}
		t.Run(tt.name, tf)
	}
}

func TestSaveInto(t *testing.T) {
	f := FakeIPTables{}

	tests := []struct {
		name   string
		table  iptables.Table
		buffer *bytes.Buffer
		expect error
	}{
		{
			"normal",
			"",
			&bytes.Buffer{},
			nil,
		},
	}

	for _, tt := range tests {
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := f.SaveInto(tt.table, tt.buffer)
				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)

			}
		}
		t.Run(tt.name, tf)
	}
}

func TestRestore(t *testing.T) {
	f := FakeIPTables{}

	tests := []struct {
		name     string
		table    iptables.Table
		data     []byte
		flush    iptables.FlushFlag
		counters iptables.RestoreCountersFlag
		expect   error
	}{
		{
			"normal",
			"",
			[]byte{},
			true,
			true,
			nil,
		},
	}

	for _, tt := range tests {
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := f.Restore(tt.table, tt.data, tt.flush, tt.counters)
				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)

			}
		}
		t.Run(tt.name, tf)
	}
}

func TestRestoreAll(t *testing.T) {
	f := FakeIPTables{}

	tests := []struct {
		name     string
		data     []byte
		flush    iptables.FlushFlag
		counters iptables.RestoreCountersFlag
		expect   error
	}{
		{
			"normal",
			[]byte{},
			true,
			true,
			nil,
		},
	}

	for _, tt := range tests {
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := f.RestoreAll(tt.data, tt.flush, tt.counters)
				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)

			}
		}
		t.Run(tt.name, tf)
	}
}

func TestMonitor(t *testing.T) {
	f := FakeIPTables{}

	tests := []struct {
		name       string
		canary     iptables.Chain
		tables     []iptables.Table
		reloadFunc func()
		interval   time.Duration
		stopCh     <-chan struct{}
		expect     error
	}{
		{
			"normal",
			"",
			[]iptables.Table{},
			func() {},
			1,
			make(<-chan struct{}),
			nil,
		},
	}

	for _, tt := range tests {
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				f.Monitor(tt.canary, tt.tables, tt.reloadFunc, tt.interval, tt.stopCh)
			}
		}
		t.Run(tt.name, tf)
	}
}

func TestGetToken(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		separator string
		expect    string
	}{
		{
			"normal",
			"a,b",
			",",
			"a",
		},
		{
			"empty",
			"",
			"",
			"",
		},
	}

	for _, tt := range tests {
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := getToken(tt.line, tt.separator)
				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)

			}
		}
		t.Run(tt.name, tf)
	}
}

func TestGetRules(t *testing.T) {
	f := FakeIPTables{
		Lines: []byte("iptables -A INPUT -p tcp --dport 22 -j ACCEPT"),
	}

	tests := []struct {
		name      string
		chainName string
		expect    []Rule
	}{
		{
			"normal",
			"INPUT",
			[]Rule{
				map[string]string{
					"--dport ": "22",
					"-j ":      "ACCEPT",
					"-p ":      "tcp",
				},
			},
		},
	}

	for _, tt := range tests {
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := f.GetRules("")
				t.Log(get)
				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)

			}
		}
		t.Run(tt.name, tf)
	}
}

func TestHasRandomFully(t *testing.T) {
	f := FakeIPTables{
		hasRandomFully: true,
	}

	tests := []struct {
		name   string
		expect bool
	}{
		{
			"normal",
			true,
		},
	}

	for _, tt := range tests {
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := f.HasRandomFully()
				t.Log(get)
				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)

			}
		}
		t.Run(tt.name, tf)
	}
}
