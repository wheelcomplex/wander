// server + client for writev test

package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/wheelcomplex/preinit/misc"
)

var exit_signal int64 = 0

var chunklen int = 4096

// max group length is 1024, limit by writev syscall
var groupNum int = 512

var runclient bool

func main() {
	var err error

	flag.BoolVar(&runclient, "c", false, "run client side")

	flag.Parse()

	var listenaddr string = "127.0.0.1:1985"

	// cpp server is running on only one cpu

	var sigChan = make(chan os.Signal, 128)
	var exitChan = make(chan os.Signal, 128)
	//
	// incoming signals will be catched
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go sigHandle(sigChan, exitChan)

	if runclient {
		// client side
		err = misc.GoHttpProfile("127.0.0.1:1902")
		if err != nil {
			fmt.Println("GoHttpProfile failed:", err)
			return
		}

		runtime.GOMAXPROCS(1)

		var err error
		clientconn, err := net.DialTimeout("tcp4", listenaddr, time.Second)
		if err != nil {
			fmt.Println("client failed:", err)
			return
		}
		client := clientconn.(*net.TCPConn)
		defer client.Close()

		//
		var totallen int = 65535

		fmt.Println(os.Getpid(), "connected ", listenaddr, "press <ctl+c> to exit ...")

		//
		readbuf := make([]byte, totallen)

		starts := time.Now()

		var n, nn int
		var totalread int
		for atomic.LoadInt64(&exit_signal) == 0 {
			n, err = client.Read(readbuf[nn:])
			if err != nil {
				fmt.Println("client failed:", err)
				break
			}
			if n > 0 {
				totalread += n
				nn += n
				if nn >= len(readbuf) {
					nn = 0
				}
			}
		}
		endts := time.Now()
		esp := endts.Sub(starts) / time.Second
		if esp <= 0 {
			esp = 1
		}
		bw := totalread / 1024 / 1024 / int(esp)
		if atomic.LoadInt64(&exit_signal) > 0 {
			fmt.Println("client exit for exit_signal ", atomic.LoadInt64(&exit_signal))
		}
		fmt.Println("client exit after", int(esp), "seconds, bandwith", bw, "MB/s.")
		return
	}

	err = misc.GoHttpProfile("127.0.0.1:1901")
	if err != nil {
		fmt.Println("GoHttpProfile failed:", err)
		return
	}

	runtime.GOMAXPROCS(1)

	// server side
	fmt.Println(os.Getpid(), "listen at", listenaddr)
	l, err := net.Listen("tcp4", listenaddr)
	if err != nil {
		fmt.Println("failed:", err)
		return
	}
	listener := l.(*net.TCPListener)
	defer listener.Close()

	go func() {
		<-exitChan
		listener.Close()
	}()

	group := make([][]byte, groupNum)

	for i := 0; i < groupNum; i++ {
		group[i] = make([]byte, chunklen)
	}

	for atomic.LoadInt64(&exit_signal) == 0 {

		var conn *net.TCPConn
		if conn, err = listener.AcceptTCP(); err != nil {
			if atomic.LoadInt64(&exit_signal) > 0 {
				fmt.Println("listener exit for signal: ", err)
				os.Exit(1)
			} else {
				panic(err)
			}
		}

		if err := conn.SetLinger(0); err != nil {
			panic(err)
		}

		// conn.SetNoDelay(false)

		for atomic.LoadInt64(&exit_signal) == 0 {
			// sendout the chunk group by writev
			if _, err := conn.Writev(group); err != nil {
				fmt.Println("failed:", err)
				break
			}
		}
		if atomic.LoadInt64(&exit_signal) > 0 {
			fmt.Println("connHandle exit for exit_signal", atomic.LoadInt64(&exit_signal))
		}

		conn.Close()

	}

}

// sigHandle
func sigHandle(sigChan, exitChan chan os.Signal) {
	sig := <-sigChan
	fmt.Println("\nsignal catched:", sig)
	atomic.AddInt64(&exit_signal, 1)
	if atomic.LoadInt64(&exit_signal) > 5 {
		os.Exit(1)
	}
	if atomic.LoadInt64(&exit_signal) > 1 {
		return
	}
	select {
	case <-exitChan:
	default:
		exitChan <- sig
		exitChan <- sig
		exitChan <- sig
		close(exitChan)
	}
}

//
