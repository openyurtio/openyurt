/*
Copyright 2023 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the License);
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an AS IS BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSetupDumpStackTrap(t *testing.T) {
	logDir := "/tmp"
	stopCh := make(chan struct{})
	defer close(stopCh)

	SetupDumpStackTrap(logDir, stopCh)

	proc, err := os.FindProcess(os.Getpid())
	assert.NoError(t, err)
	assert.NoError(t, proc.Signal(syscall.SIGUSR1))

	// Wait for a short time to allow stack dump to complete
	time.Sleep(time.Millisecond * 100)

	fileName := fmt.Sprintf("yurthub.%d.stacks.log", os.Getpid())
	filePath := filepath.Join(logDir, fileName)

	assert.FileExists(t, filePath)
	assert.NoError(t, os.Remove(filePath))
}
