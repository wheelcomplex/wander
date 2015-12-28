package main

import (
	"fmt"
	"net"
	"os"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	_ "net/http/pprof"
)

// for debug
var tpf = fmt.Printf
var totalmutex sync.RWMutex
var totalcount int
var gBuffer = make([]byte, 1)

func main() {
	cpus := runtime.GOMAXPROCS(-1) / 2
	if cpus <= 0 {
		cpus = 1
	}
	runtime.GOMAXPROCS(cpus)

	//	profileurl := "localhost:6092"
	//	fmt.Printf("profile: [http://%s/debug/pprof/]\nhttp://%s/debug/pprof/\n", profileurl, profileurl)
	//	go func() {
	//		fmt.Println(http.ListenAndServe(profileurl, nil))
	//	}()

	laddr, err := net.ResolveTCPAddr("tcp", "0.0.0.0:5180")
	if err != nil {
		tpf("%s\n", err.Error())
		return
	}
	l, err := net.ListenTCP("tcp", laddr)
	if err != nil {
		tpf("%s\n", err.Error())
		return
	}

	var wg sync.WaitGroup
	go acceptor(laddr, l, &wg)
	for idx := 0; idx < runtime.GOMAXPROCS(-1); idx++ {
		wg.Add(1)
		go acceptor(laddr, nil, &wg)

	}
	tpf("[%d] %d worker listening %s ...\n", os.Getpid(), runtime.GOMAXPROCS(-1)+1, laddr.String())
	closed := make(chan struct{})
	go stat(closed)
	wg.Wait()
	close(closed)
	tpf("exited.\n")
}

func acceptor(laddr *net.TCPAddr, l *net.TCPListener, wg *sync.WaitGroup) {
	defer wg.Done()
	var err error
	if l == nil {
		l, err = net.ListenTCP("tcp", laddr)
		if err != nil {
			tpf("%s\n", err.Error())
			return
		}
	}
	for {
		conn, err := l.AcceptTCP()
		if err != nil {
			tpf("%s\n", err.Error())
			break
		}
		totalmutex.Lock()
		wg.Add(1)
		go readconn(totalcount, conn, wg)
		totalcount++
		totalmutex.Unlock()
	}
	l.Close()
}

func stat(closed chan struct{}) {
	tk := time.NewTicker(10 * time.Second)
	defer tk.Stop()
	var pre int
	for {
		totalmutex.RLock()
		if pre != totalcount {
			tpf("totalcount: %d\n", totalcount)
			pre = totalcount
		}
		if totalcount == 0 {
			debug.FreeOSMemory()
		}
		totalmutex.RUnlock()
		<-tk.C
		select {
		case <-closed:
			return
		default:
		}
	}
}

func readconn(idx int, conn *net.TCPConn, wg *sync.WaitGroup) {
	var err error = nil
	for err == nil {
		_, err = conn.Read(gBuffer)
	}
	//tpf("readconn#%d, exited for %s\n", idx, err.Error())
	totalmutex.Lock()
	totalcount--
	totalmutex.Unlock()
	wg.Done()
	conn.Close()
}
