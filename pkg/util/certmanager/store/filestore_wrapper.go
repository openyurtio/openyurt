/*
Copyright 2021 The OpenYurt Authors.

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

package store

import (
	"crypto/tls"

	"k8s.io/client-go/util/certificate"
	"k8s.io/klog/v2"
)

// fileStoreWrapper is a wrapper for "k8s.io/client-go/util/certificate#FileStore"
// This wrapper increases tolerance for unexpected situations and is more robust.
type fileStoreWrapper struct {
	certificate.FileStore
}

// NewFileStoreWrapper returns a wrapper for "k8s.io/client-go/util/certificate#FileStore"
// This wrapper increases tolerance for unexpected situations and is more robust.
func NewFileStoreWrapper(pairNamePrefix, certDirectory, keyDirectory, certFile, keyFile string) (certificate.FileStore, error) {
	fileStore, err := certificate.NewFileStore(pairNamePrefix, certDirectory, keyDirectory, certFile, keyFile)
	if err != nil {
		return nil, err
	}
	return &fileStoreWrapper{
		FileStore: fileStore,
	}, nil
}

func (s *fileStoreWrapper) Current() (*tls.Certificate, error) {
	cert, err := s.FileStore.Current()
	// If an error occurs, just return the NoCertKeyError.
	// The cert-manager will regenerate the related certificates when it receives the NoCertKeyError.
	if err != nil {
		klog.Warningf("unexpected error occurred when loading the certificate: %v, will regenerate it", err)
		noCertKeyErr := certificate.NoCertKeyError("NO_VALID_CERT")
		return nil, &noCertKeyErr
	}
	return cert, nil
}
