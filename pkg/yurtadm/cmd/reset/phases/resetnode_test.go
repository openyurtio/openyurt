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

package phases

import (
	"errors"
	"testing"
)

func Test_runStopYurthubService_Success(t *testing.T) {
	oldStop := stopYurthubServiceFunc
	oldDisable := disableYurthubServiceFunc
	defer func() {
		stopYurthubServiceFunc = oldStop
		disableYurthubServiceFunc = oldDisable
	}()

	stopYurthubServiceFunc = func() error { return nil }
	disableYurthubServiceFunc = func() error { return nil }

	if err := runStopYurthubService(); err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func Test_runStopYurthubService_StopFails(t *testing.T) {
	oldStop := stopYurthubServiceFunc
	oldDisable := disableYurthubServiceFunc
	defer func() {
		stopYurthubServiceFunc = oldStop
		disableYurthubServiceFunc = oldDisable
	}()

	stopYurthubServiceFunc = func() error { return errors.New("stop failed") }
	disableYurthubServiceFunc = func() error { return nil }

	if err := runStopYurthubService(); err == nil {
		t.Fatalf("expected error when stop fails, got nil")
	}
}

func Test_runStopYurthubService_DisableFails(t *testing.T) {
	oldStop := stopYurthubServiceFunc
	oldDisable := disableYurthubServiceFunc
	defer func() {
		stopYurthubServiceFunc = oldStop
		disableYurthubServiceFunc = oldDisable
	}()

	stopYurthubServiceFunc = func() error { return nil }
	disableYurthubServiceFunc = func() error { return errors.New("disable failed") }

	if err := runStopYurthubService(); err == nil {
		t.Fatalf("expected error when disable fails, got nil")
	}
}
