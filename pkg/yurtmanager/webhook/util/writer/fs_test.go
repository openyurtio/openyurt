/*
Copyright 2026 The OpenYurt Authors.

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

package writer

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/openyurtio/openyurt/pkg/yurtmanager/webhook/util/generator"
)

func TestCertToProjectionMapModes(t *testing.T) {
	certs := &generator.Artifacts{
		CAKey:  []byte("ca-key"),
		CACert: []byte("ca-cert"),
		Cert:   []byte("cert"),
		Key:    []byte("key"),
	}

	projections := certToProjectionMap(certs)

	tests := []struct {
		name         string
		file         string
		expectedMode int32
	}{
		{name: "CA private key is not group or world readable", file: CAKeyName, expectedMode: keyFileMode},
		{name: "server private key is not group or world readable", file: ServerKeyName, expectedMode: keyFileMode},
		{name: "server private key alias is not group or world readable", file: ServerKeyName2, expectedMode: keyFileMode},
		{name: "CA certificate is world readable", file: CACertName, expectedMode: certFileMode},
		{name: "server certificate is world readable", file: ServerCertName, expectedMode: certFileMode},
		{name: "server certificate alias is world readable", file: ServerCertName2, expectedMode: certFileMode},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projection, ok := projections[tt.file]
			require.Truef(t, ok, "no projection for %s", tt.file)
			require.Equalf(t, tt.expectedMode, projection.Mode, "unexpected mode for %s", tt.file)
		})
	}

	// The writer calls os.Chmod with these modes explicitly, so they are applied
	// regardless of the process umask. No file may be writable by group or other.
	for file, projection := range projections {
		require.Zerof(t, projection.Mode&0022, "%s is group or world writable (mode %#o)", file, projection.Mode)
	}
}

func TestCertDirModeIsNotWorldWritable(t *testing.T) {
	require.Zerof(t, certDirMode&0022, "cert directory is group or world writable (mode %#o)", certDirMode)
}
