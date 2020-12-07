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

package server

import (
	"bytes"
	"io"
)

var (
	TLS_STRING = []byte("http: TLS handshake error from")
)

type HandshakeFilterWriter struct {
	io.Writer
}

func NewHandshakeFilterWriter(w io.Writer) io.Writer {
	return &HandshakeFilterWriter{
		Writer: w,
	}
}

func (w *HandshakeFilterWriter) Write(b []byte) (n int, err error) {
	if bytes.Contains(b, TLS_STRING) {
		return 0, nil
	}
	return w.Writer.Write(b)
}
