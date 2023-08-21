/*
Copyright 2023 The OpenYurt Authors.

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

type notFoundError struct {
	err error
}

func (e notFoundError) Error() string {
	return e.err.Error()
}

type alreadyExistError struct {
	err error
}

func (e alreadyExistError) Error() string {
	return e.err.Error()
}

type notExistError struct {
	err error
}

func (e notExistError) Error() string {
	return e.err.Error()
}

func isNotExist(err error) bool {
	_, ok := err.(notExistError)
	return ok
}
