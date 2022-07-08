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

package fs

import "errors"

var (
	ErrIsNotDir    = errors.New("the path is not a directory")
	ErrIsNotFile   = errors.New("the path is not a regular file")
	ErrExists      = errors.New("path has already existed")
	ErrNotExists   = errors.New("path does not exist")
	ErrInvalidPath = errors.New("invalid path")
)
