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

package util

import "strconv"

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

func FloatToStr(num *float64) string {
	var res string
	if num != nil {
		res = strconv.FormatFloat(*num, 'f', -1, 64)
	} else {
		res = ""
	}
	return res
}

func UintToStr(num *uint64) string {
	var res string
	if num != nil {
		res = strconv.FormatUint(*(num), 10)
	} else {
		res = ""
	}
	return res
}

func IntToStr(num *int64) string {
	var res string
	if num != nil {
		res = strconv.FormatInt(*(num), 10)
	} else {
		res = ""
	}
	return res
}

func StrToFloat(str string) *float64 {
	num, err := strconv.ParseFloat(str, 64)
	if err != nil {
		return nil
	}
	return &num
}

func StrToUint(str string) *uint64 {
	num, err := strconv.ParseUint(str, 10, 64)
	if err != nil {
		return nil
	}
	return &num
}

func StrToInt(str string) *int64 {
	num, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		return nil
	}
	return &num
}
