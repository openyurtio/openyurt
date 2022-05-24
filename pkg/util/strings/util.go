/*
Copyright 2020 The OpenYurt Authors.

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

package strings

// IsInStringLst checks if 'str' is in the 'strLst'
func IsInStringLst(strLst []string, str string) bool {
	if len(strLst) == 0 {
		return false
	}
	for _, s := range strLst {
		if str == s {
			return true
		}
	}
	return false
}
