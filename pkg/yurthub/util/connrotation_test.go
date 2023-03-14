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

package util

import (
	"io"
	"net"
	"testing"
	"time"
)

func TestNewDialer(t *testing.T) {
	dialerName := "testDialer"
	localAddress := ":13999"
	notExistLocalAddress := ":13998"
	network := "tcp"
	requestMessage := []byte("some message")
	stopCh := make(chan struct{})

	defer close(stopCh)

	go func(stopCh <-chan struct{}) {
		l, err := net.Listen(network, localAddress)
		if err != nil {
			t.Error(err)
			return
		}
		defer l.Close()
		for {
			select {
			case <-stopCh:
				return
			default:
			}

			conn, err := l.Accept()
			if err != nil {
				return
			}
			defer conn.Close()

			_, err = io.ReadAll(conn)
			if err != nil {
				t.Error(err)
				return
			}

		}
	}(stopCh)

	time.Sleep(time.Second)

	d := NewDialer(dialerName)
	if d == nil {
		t.Error("NewDialer return nil")
		return
	}
	if d.Name() != dialerName {
		t.Errorf("dailer name mismatch. want:%s, got:%s", dialerName, d.Name())
		return
	}

	_, err := d.Dial(network, notExistLocalAddress)
	if err == nil {
		t.Error("dail not exist address. want err, got:nil")
		return
	}

	conn1, err := d.Dial(network, localAddress)
	if err != nil {
		t.Errorf("dail exist address. want:nil err, got:%v", err)
		return
	}
	if _, err = conn1.Write(requestMessage); err != nil {
		t.Errorf("write message. err:%v", err)
		return
	}
	d.Close(localAddress)
	if err = conn1.Close(); err == nil {
		t.Error("after closing all connections, single connection can still be closed")
		return
	}

	conn2, err := d.Dial(network, localAddress)
	if err != nil {
		t.Errorf("dail exist address. want:nil err, got:%v", err)
		return
	}
	if _, err = conn2.Write(requestMessage); err != nil {
		t.Errorf("write message. err:%v", err)
		return
	}

	d.CloseAll()
	if err = conn2.Close(); err == nil {
		t.Error("after closing all connections, single connection can still be closed")
		return
	}

}
