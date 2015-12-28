// io.Copy, funny's splice vs runtime sendfile

package main

import (
	"bytes"
	"flag"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"runtime"
	"time"

	"github.com/wheelcomplex/wander/netsplice-echo-proxy/splicetest/funny"
	"github.com/wheelcomplex/wander/netsplice-echo-proxy/splicetest/normal"
)

var testflag int

var t = log.New(os.Stderr, "splice test ", log.LstdFlags)

func main() {

	flag.IntVar(&testflag, "t", 0, "test select, 0 for runtime + funny, 1 for funny, 2 for runtime")
	flag.Parse()

	pid := os.Getpid()

	t.Printf("[%d] test flag %d, with %d CPUs.\n", pid, testflag, runtime.GOMAXPROCS(0))

	// Setup a echo server
	backendLsn, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	defer backendLsn.Close()
	go func() {
		// backend echo server running
		conn, err := backendLsn.Accept()
		if err != nil {
			panic(err)
		}
		conn.(*net.TCPConn).SetNoDelay(true)
		io.Copy(conn, conn)
	}()

	var funnyLsn net.Listener
	var normalLsn net.Listener

	if testflag == 0 || testflag == 1 {
		// Setup a funnyproxy server
		funnyLsn, err = net.Listen("tcp", "0.0.0.0:0")
		if err != nil {
			t.Fatal(err)
		}
		defer funnyLsn.Close()
		go func() {
			t.Printf("funnyproxy server running: %s\n", funnyLsn.(*net.TCPListener).Addr().String())
			conn, err := funnyLsn.Accept()
			if err != nil {
				panic(err)
			}
			defer conn.Close()
			conn.(*net.TCPConn).SetNoDelay(true)

			// Dial to backend echo server
			agent, err := net.Dial("tcp", backendLsn.Addr().String())
			if err != nil {
				panic(err)
			}
			defer agent.Close()
			agent.(*net.TCPConn).SetNoDelay(true)

			// Begin transfer
			proxy := funny.NewProxy()
			t.Printf("funnyproxy server Transfer: %s <=> %s\n", conn.LocalAddr().String(), conn.RemoteAddr().String())
			proxy.Transfer(conn, agent)
		}()
	}
	if testflag == 0 || testflag == 2 {
		// Setup a normalproxy server
		normalLsn, err = net.Listen("tcp", "0.0.0.0:0")
		if err != nil {
			t.Fatal(err)
		}
		defer normalLsn.Close()
		go func() {
			t.Printf("normalproxy server running: %s\n", normalLsn.(*net.TCPListener).Addr().String())
			conn, err := normalLsn.Accept()
			if err != nil {
				panic(err)
			}
			defer conn.Close()
			conn.(*net.TCPConn).SetNoDelay(true)

			// Dial to backend echo server
			agent, err := net.Dial("tcp", backendLsn.Addr().String())
			if err != nil {
				panic(err)
			}
			defer agent.Close()
			agent.(*net.TCPConn).SetNoDelay(true)

			// Begin transfer
			proxy := normal.NewProxy()
			t.Printf("normalproxy server Transfer: %s <=> %s\n", conn.LocalAddr().String(), conn.RemoteAddr().String())
			proxy.Transfer(conn, agent)
		}()
	}

	var maxloop int = 100000
	var x int = 0
	if testflag == 0 || testflag == 2 {

		conn, err := net.Dial("tcp", normalLsn.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		t.Printf("connected to normal proxy, loop %d time: %s\n", maxloop, conn.(*net.TCPConn).RemoteAddr().String())
		conn.(*net.TCPConn).SetNoDelay(true)

		startts := time.Now()
		x = 0
		for i := 0; i < maxloop; i++ {
			//println()
			b1 := RandBytes(256)
			b2 := make([]byte, len(b1))
			x += len(b1)
			// println(x)

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
		esp := time.Now().Sub(startts)
		if esp <= 0 {
			esp = 1
		}
		bw := int((time.Duration(maxloop) * time.Second) / esp)
		t.Printf("normal test done, esp %v, total %d, %d/s\n", esp, maxloop, bw)
	}
	if testflag == 0 || testflag == 1 {

		conn, err := net.Dial("tcp", funnyLsn.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		t.Printf("connected to funny proxy, loop %d time: %s\n", maxloop, conn.(*net.TCPConn).RemoteAddr().String())
		conn.(*net.TCPConn).SetNoDelay(true)

		startts := time.Now()
		x = 0
		for i := 0; i < maxloop; i++ {
			//println()
			b1 := RandBytes(256)
			b2 := make([]byte, len(b1))
			x += len(b1)
			println(x)

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
		esp := time.Now().Sub(startts)
		if esp <= 0 {
			esp = 1
		}
		bw := int((time.Duration(maxloop) * time.Second) / esp)
		t.Printf("funny test done, esp %v, total %d, %d/s\n", esp, maxloop, bw)
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
