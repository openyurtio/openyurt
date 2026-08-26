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

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"k8s.io/klog/v2"
)

// SetupDumpStackTrap sets up a goroutine that listens for stop signals
// to dump goroutine stacks.
func SetupDumpStackTrap(logDir string, stopCh <-chan struct{}) {
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		select {
		case <-stopCh:
			return
		case <-time.After(1 * time.Second):
			dumpStacks(false, logDir)
			return
		}
	}()

	select {
	case <-stopCh:
	case <-time.After(100 * time.Millisecond):
	}
	wg.Wait()
}

// dumpStacks writes the current goroutine stacks to a file or logs them.
func dumpStacks(writeToFile bool, logDir string) {
	var (
		buf       []byte
		stackSize int
	)
	bufferLen := 16384
	for stackSize == len(buf) {
		buf = make([]byte, bufferLen)
		stackSize = runtime.Stack(buf, true)
		bufferLen *= 2
	}
	buf = buf[:stackSize]
	klog.Infof("=== BEGIN goroutine stack dump ===\n%s\n=== END goroutine stack dump ===", buf)

	if writeToFile {
		if err := os.MkdirAll(logDir, 0755); err != nil {
			klog.Errorf("failed to create directory %s for stack dump: %v", logDir, err)
			return
		}

		name := filepath.Join(logDir, fmt.Sprintf("yurthub.%d.stacks.log", os.Getpid()))
		if err := os.WriteFile(name, buf, 0644); err != nil {
			klog.Errorf("failed to write stack dump to %s: %v", name, err)
		} else {
			klog.Infof("goroutine stack dump written to %s", name)
		}
	}
}
