// +build !linux

package lab04

import (
	"io"
	"net"
)

type Proxy struct{}

func newProxy() Proxy {
	return Proxy{}
}

func (p Proxy) Transfer(conn1, conn2 net.Conn) error {
	errChan := make(chan error, 1)
	go func() {
		_, err := io.Copy(conn2, conn1)
		conn1.Close()
		conn2.Close()
		errChan <- err
	}()

	_, err1 := io.Copy(conn1, conn2)
	conn1.Close()
	conn2.Close()
	err2 := <-errChan

	if err1 != nil {
		return err1
	}
	return err2
}
