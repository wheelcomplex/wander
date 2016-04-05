//

// server + client for tcpconn nonblocking read/write test

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

	"lowlevel/extnet"
)

var exit_signal int64 = 0

var chunklen int = 4096

// max group length is 1024, limit by writev syscall
var groupNum int = 512

var runclient bool

func main() {
	// pause befor panic exit
	runtime.SetPanicFlag(2)

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
	//
	var totallen int = 65535

	if runclient {
		// client side
		err = extnet.GoHttpProfile("127.0.0.1:1902")
		if err != nil {
			fmt.Println("GoHttpProfile failed:", err)
			return
		}

		//runtime.GOMAXPROCS(1)

		var err error
		clientconn, err := net.DialTimeout("tcp4", listenaddr, time.Second)
		if err != nil {
			fmt.Println("client failed:", err)
			return
		}
		client := clientconn.(*net.TCPConn)
		defer client.Close()

		fmt.Println(os.Getpid(), "connected ", listenaddr, "press <ctl+c> to exit ...")

		//
		readbuf := make([]byte, totallen)

		connHandle(client, readbuf, "read")

		fmt.Println("client exited.")
		return
	}

	err = extnet.GoHttpProfile("127.0.0.1:1901")
	if err != nil {
		fmt.Println("GoHttpProfile failed:", err)
		return
	}

	// runtime.GOMAXPROCS(1)

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

	//
	writebuf := make([]byte, totallen)

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

		/*
			if err := conn.SetLinger(0); err != nil {
				panic(err)
			}
		*/

		// conn.SetNoDelay(false)

		connHandle(conn, writebuf, "write")

		if atomic.LoadInt64(&exit_signal) > 0 {
			fmt.Println("Listener exit for exit_signal", atomic.LoadInt64(&exit_signal))
		}

	}
	fmt.Println("server exited.")
	return
}

func connHandle(conn *net.TCPConn, buf []byte, mode string) {
	var reading bool
	if mode == "read" {
		reading = true
	}
	defer conn.Close()

	evchan, err := conn.EventChanNB()
	if err != nil {
		fmt.Println(mode, " get event channel failed:", err)
		return
	}

	starts := time.Now()
	var n, nn int
	var totalio int
	for atomic.LoadInt64(&exit_signal) == 0 {
		event, ok := <-evchan
		if !ok {
			fmt.Println(mode, "event closed.")
			break
		}
		if reading == true {
			if event != runtime.FDEV_READ {
				fmt.Println(mode, "warning: need FDEV_READ", runtime.FDEV_READ, "got", event)
				continue
			}
			n, err = conn.ReadNB(buf[nn:])
			if err != nil {
				fmt.Println(mode, "failed:", err)
				break
			}
		}
		if reading == false {
			if event != runtime.FDEV_WRITE {
				fmt.Println(mode, "warning: need FDEV_WRITE", runtime.FDEV_WRITE, "got", event)
				continue
			}
			n, err = conn.WriteNB(buf[nn:])
			if err != nil {
				fmt.Println(mode, "failed:", err)
				break
			}
		}
		if n > 0 {
			totalio += n
			nn += n
			if nn >= len(buf) {
				nn = 0
			}
		}
	}
	endts := time.Now()
	esp := endts.Sub(starts) / time.Second
	if esp <= 0 {
		esp = 1
	}
	bw := totalio / 1024 / 1024 / int(esp)
	if atomic.LoadInt64(&exit_signal) > 0 {
		fmt.Println(mode, "exit for exit_signal ", atomic.LoadInt64(&exit_signal))
	}
	fmt.Println(mode, "exit after", int(esp), "seconds, bandwith", bw, "MB/s.")
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
