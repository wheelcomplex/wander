package lab04

import (
	"bytes"
	"io"
	"math/rand"
	"net"
	"testing"
)

func Test_Proxy(t *testing.T) {
	// Setup a echo server
	backendLsn, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		conn, err := backendLsn.Accept()
		if err != nil {
			panic(err)
		}
		conn.(*net.TCPConn).SetNoDelay(true)
		io.Copy(conn, conn)
	}()

	// Setup a proxy server
	proxyLsn, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		conn, err := proxyLsn.Accept()
		if err != nil {
			panic(err)
		}
		conn.(*net.TCPConn).SetNoDelay(true)

		// Dial to backend echo server
		agent, err := net.Dial("tcp", backendLsn.Addr().String())
		if err != nil {
			panic(err)
		}
		agent.(*net.TCPConn).SetNoDelay(true)

		// Begin transfer
		proxy := newProxy()
		proxy.Transfer(conn, agent)
	}()

	conn, err := net.Dial("tcp", proxyLsn.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	conn.(*net.TCPConn).SetNoDelay(true)

	x := 0
	for i := 0; i < 100000; i++ {
		//println()
		b1 := RandBytes(256)
		b2 := make([]byte, len(b1))
		x += len(b1)
		//println(x)

		//println("write", len(b1))
		_, err := conn.Write(b1)
		if err != nil {
			t.Fatal(err)
		}

		//println("read")
		_, err = io.ReadFull(conn, b2)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(b1, b2) {
			t.Fatal()
		}
	}
}

func RandBytes(n int) []byte {
	n = rand.Intn(n) + 1
	b := make([]byte, n)
	for i := 0; i < n; i++ {
		b[i] = byte(rand.Intn(255))
	}
	return b
}
