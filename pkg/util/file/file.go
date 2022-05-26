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

package file

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientsetscheme "k8s.io/client-go/kubernetes/scheme"
)

// FileExists checks if specified file exists.
func FileExists(filename string) (bool, error) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

func ReadObjectFromYamlFile(path string) (runtime.Object, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read object from yaml file(%s) with error: %v", path, err)
	}

	const mediaType = runtime.ContentTypeYAML
	info, ok := runtime.SerializerInfoForMediaType(clientsetscheme.Codecs.SupportedMediaTypes(), mediaType)
	if !ok {
		return nil, fmt.Errorf("unsupported media type %q", mediaType)
	}

	decoder := clientsetscheme.Codecs.DecoderToVersion(info.Serializer, v1.SchemeGroupVersion)
	return runtime.Decode(decoder, buf)
}

func WriteObjectToYamlFile(obj runtime.Object, path string) error {
	const mediaType = runtime.ContentTypeYAML
	info, ok := runtime.SerializerInfoForMediaType(clientsetscheme.Codecs.SupportedMediaTypes(), mediaType)
	if !ok {
		return fmt.Errorf("unsupported media type %q", mediaType)
	}

	encoder := clientsetscheme.Codecs.EncoderForVersion(info.Serializer, v1.SchemeGroupVersion)
	buf, err := runtime.Encode(encoder, obj)
	if err != nil {
		return fmt.Errorf("failed to encode object, %v", err)
	}

	tmpPath := fmt.Sprintf("%s.tmp", path)
	if err := os.WriteFile(tmpPath, buf, 0600); err != nil {
		return fmt.Errorf("failed to write object into manifest file(%s), %v", tmpPath, err)
	}

	if err := backupFile(path); err != nil {
		os.Remove(tmpPath)
		return err
	}

	if err := os.Remove(path); err != nil {
		os.Remove(tmpPath)
		return err
	}

	// rename tmp path file to path file
	return os.Rename(tmpPath, path)
}

func backupFile(path string) error {
	src, err := os.Open(path)
	if err != nil {
		return err
	}

	fileName := filepath.Base(path)
	bakFile := filepath.Join("/tmp", fileName)
	dst, err := os.Create(bakFile)
	if err != nil {
		return err
	}

	_, err = io.Copy(dst, src)
	return err
}
