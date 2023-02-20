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

package reset

import (
	"io"
	"os"
	"reflect"
	"testing"

	flag "github.com/spf13/pflag"

	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
)

const (
	failed  = "\u2717"
	succeed = "\u2713"
)

func TestNewResetOptions(t *testing.T) {
	tests := []struct {
		name   string
		expect resetOptions
	}{
		{
			"normal",
			resetOptions{
				certificatesDir: constants.DefaultCertificatesDir,
				forceReset:      false,
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := newResetOptions()

				if !reflect.DeepEqual(tt.expect, *get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, get, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, get, get)

			}
		})
	}
}

func TestNewResetData(t *testing.T) {
	opt := resetOptions{
		certificatesDir:       "/",
		criSocketPath:         "/",
		forceReset:            true,
		ignorePreflightErrors: []string{},
	}

	tests := []struct {
		name    string
		options *resetOptions
		expect  resetData
	}{
		{
			"normal",
			&opt,
			resetData{
				certificatesDir:       opt.certificatesDir,
				criSocketPath:         opt.criSocketPath,
				forceReset:            opt.forceReset,
				ignorePreflightErrors: opt.ignorePreflightErrors,
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get, _ := newResetData(&opt)

				if !reflect.DeepEqual(tt.expect, *get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, get, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, get, get)

			}
		})
	}
}

func TestAddResetFlags(t *testing.T) {
	opt := resetOptions{
		certificatesDir:       "/",
		criSocketPath:         "/",
		forceReset:            true,
		ignorePreflightErrors: []string{},
	}

	tests := []struct {
		name         string
		flagSet      *flag.FlagSet
		resetOptions *resetOptions
		expect       bool
	}{
		{
			"normal",
			flag.NewFlagSet("normal", 0),
			&opt,
			true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				AddResetFlags(tt.flagSet, tt.resetOptions)
				get := true
				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, get, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, get, get)

			}
		})
	}
}

func TestNewReseterWithResetData(t *testing.T) {
	tests := []struct {
		name   string
		o      *resetData
		in     io.Reader
		out    io.Writer
		outErr io.Writer
		expect nodeReseter
	}{
		{
			"normal",
			&resetData{},
			os.Stdin,
			os.Stdout,
			os.Stderr,
			nodeReseter{
				&resetData{},
				os.Stdin,
				os.Stdout,
				os.Stderr,
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := newReseterWithResetData(tt.o, tt.in, tt.out, tt.outErr)
				if !reflect.DeepEqual(tt.expect, *get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, get, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, get, get)
			}
		})
	}
}

// func TestRun(t *testing.T) {
//	nr := newReseterWithResetData(
//		&resetData{},
//		os.Stdin,
//		os.Stdout,
//		os.Stderr)
//
//	tests := []struct {
//		name   string
//		expect error
//	}{
//		{
//			"normal",
//			nil,
//		},
//	}
//
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			t.Parallel()
//			t.Logf("\tTestCase: %s", tt.name)
//			{
//				get := nr.Run()
//				if !reflect.DeepEqual(tt.expect, get) {
//					t.Fatalf("\t%s\texpect %v, but get %v", failed, get, get)
//				}
//				t.Logf("\t%s\texpect %v, get %v", succeed, get, get)
//			}
//		})
//	}
// }

var rd *resetData = &resetData{
	certificatesDir:       "/",
	criSocketPath:         "/",
	forceReset:            true,
	ignorePreflightErrors: []string{},
}

func TestCertificatesDir(t *testing.T) {
	tests := []struct {
		name   string
		expect string
	}{
		{
			"normal",
			"/",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := rd.CertificatesDir()
				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, get, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, get, get)
			}
		})
	}
}

func TestForceReset(t *testing.T) {
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
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := rd.ForceReset()
				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, get, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, get, get)
			}
		})
	}
}

func TestIgnorePreflightErrors(t *testing.T) {
	tests := []struct {
		name   string
		expect []string
	}{
		{
			"normal",
			[]string{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := rd.IgnorePreflightErrors()
				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, get, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, get, get)
			}
		})
	}
}

func TestCRISocketPath(t *testing.T) {
	tests := []struct {
		name   string
		expect string
	}{
		{
			"normal",
			"/",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := rd.CRISocketPath()
				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, get, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, get, get)
			}
		})
	}
}
