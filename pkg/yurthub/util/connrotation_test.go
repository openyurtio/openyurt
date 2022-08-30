package util

import (
	"io/ioutil"
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
			t.Fatal(err)
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

			_, err = ioutil.ReadAll(conn)
			if err != nil {
				t.Fatal(err)
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
