//
// a tcp server/client engine for testing
// max accept speed: 88k/s, connector(-c 1000000 -t 4) and acceptor(-c 1000000 -t 4 -l) in the same laptop

// laptop: asus n500jk Intel(R) Core(TM) i7-4700HQ CPU @ 2.40GHz 8 cores, 16GB RAM, ubuntu 15.04 3.19.0-26-lowlatency x86_64

package main

import (
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"flag"
	"fmt"
	"math"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"net/http"
	_ "net/http/pprof"
)

func httpProfile(profileurl string) {
	binpath, _ := filepath.Abs(os.Args[0])
	fmt.Printf("\n http profile: [ http://%s/debug/pprof/ ]\n", profileurl)
	fmt.Printf("\n http://%s/debug/pprof/goroutine?debug=1\n\n", profileurl)
	fmt.Printf("\ngo tool pprof %s http://%s/debug/pprof/profile\n\n", binpath, profileurl)
	fmt.Printf("\ngo tool pprof %s http://%s/debug/pprof/heap\n\n", binpath, profileurl)
	go func() {
		if err := http.ListenAndServe(profileurl, nil); err != nil {
			fmt.Printf("http/pprof: %s\n", err.Error())
			os.Exit(1)
		}
	}()
}

//
func main() {

	notifys := []os.Signal{syscall.SIGCHLD, syscall.SIGPIPE}
	//exits := []os.Signal{syscall.SIGABRT, syscall.SIGINT, syscall.SIGPIPE, syscall.SIGQUIT, syscall.SIGTERM}
	// don't use SIGQUIT
	exits := []os.Signal{syscall.SIGABRT, syscall.SIGINT, syscall.SIGTERM}

	conf := NewCmdConfig(notifys, exits)

	// --------------------------- //

	eng := NewEngine(conf)

	if err := eng.Start(); err != nil {
		fmt.Printf("start %s\n", err.Error())
		os.Exit(1)
	}

	//
	eng.Wait()
	// end
}

//
const NET_MICRO_DELAY time.Duration = 200 * time.Millisecond

//
const NET_CONNECT_TIMEOUT time.Duration = 1 * time.Second

//
const NET_FORCE_EXIT int = 5

//
var chanSignal = struct{}{}

//
// tcp Engine
type Engine struct {
	conf *Config // Engine config

	// for GC map scan test
	conns map[uint64]*net.TCPConn // huge map for test, in real program, it's a more complicate map value for business logic

	ivbuf      []byte             // iv for aes work load
	aeskey     []byte             // aes key
	dummyBuf   []byte             // dummy buffer, share by all connections for read/write/aes work load
	aliveCount *int64             // running counter
	newCount   *int64             // accept/dial counter
	rwCount    *int64             // conn read/write counter
	laddrList  []net.TCPAddr      // local TCPAddr for connector
	laddrPool  []chan uint64      // local addr index
	maxCond    *sync.Cond         // locker for connections counter
	maxLock    *sync.RWMutex      // locker for connections counter
	workswitch chan struct{}      // work load switch
	workloadon bool               // work load switch flag
	qpschannel chan struct{}      // qps ticks channel
	listenfds  []*net.TCPListener // save listener for close
	binfile    string             //
	cpufile    string             //
	heapstart  string             //
	heapend    string             //
	pprofbase  string             //
	sigChan    chan os.Signal     // os signal handle
	sigList    []os.Signal        // signal list for handle
	closing    bool               // closing flag for engine manage
	closed     bool               // closed flag for engine manage
	destroyed  chan struct{}      // Engine closed signal
	m          sync.Mutex         // locker for engine manage
	wg         sync.WaitGroup     // acceptor/connector sync
}

//
func NewEngine(conf *Config) *Engine {
	// initial counter
	var newcc, rwcc, aliveCount int64
	maxLock := &sync.RWMutex{}
	eng := &Engine{
		conf:       conf,
		ivbuf:      []byte("4e681a397ab93cba"),
		aeskey:     []byte("397ab4e3cba681a9"),
		conns:      make(map[uint64]*net.TCPConn),
		dummyBuf:   make([]byte, aes.BlockSize*1),
		aliveCount: &aliveCount,
		newCount:   &newcc,
		rwCount:    &rwcc,
		laddrPool:  make([]chan uint64, 0),
		maxLock:    maxLock,
		maxCond:    sync.NewCond(maxLock),
		workswitch: make(chan struct{}, 0),
		listenfds:  make([]*net.TCPListener, 0, conf.cpus),
		sigChan:    make(chan os.Signal, 128),
		sigList:    make([]os.Signal, 0),
		destroyed:  make(chan struct{}),
	}
	if len(eng.conf.notifys) > 0 {
		for _, sig := range eng.conf.notifys {
			eng.sigList = append(eng.sigList, sig)
		}
	}
	if len(eng.conf.exits) > 0 {
		for _, sig := range eng.conf.exits {
			eng.sigList = append(eng.sigList, sig)
		}
	}
	eng.binfile, _ = filepath.Abs(os.Args[0])
	return eng
}

//
func (eng *Engine) Start() error {

	destAddr, err := net.ResolveTCPAddr("tcp", eng.conf.hostport)
	if err != nil {
		fmt.Printf("%s\n", err.Error())
		return err
	}

	var idx int

	//
	if len(eng.sigList) > 0 {
		// BUG: using signal.Notify(eng.sigChan) and signal.Reset(syscall.SIGQUIT) will case Quit (core dumped)
		signal.Notify(eng.sigChan, eng.sigList...)
		go eng.sigHandle()
	}

	if eng.conf.listenMode == false {

		// this is connector, client side

		if eng.conf.cpus == 0 {
			eng.conf.cpus = runtime.GOMAXPROCS(-1)/2 + 1
		}

		runtime.GOMAXPROCS(eng.conf.cpus)
		eng.conf.cpus = runtime.GOMAXPROCS(-1)

		fmt.Printf("[%d] %d worker connecting to %s, max concurrent %d, max RW QPS %d, press <CTL-C> to exit ...\n", os.Getpid(), eng.conf.cpus, destAddr.String(), eng.conf.maxConcurrent, eng.conf.QPSLimit)

		eng.laddrList = eng.addressParse(eng.conf.laddrlist)
		if len(eng.laddrList) == 0 {
			fmt.Printf("invalid local address for connector: %s\n", eng.conf.laddrlist)
			os.Exit(1)
		}

		fmt.Printf("loading local address: %d ...\n", len(eng.laddrList))

		// pre-load
		// +cpus for never block
		avglen := len(eng.laddrList)/eng.conf.cpus + eng.conf.cpus
		for idx = 0; idx < eng.conf.cpus; idx++ {
			eng.laddrPool = append(eng.laddrPool, make(chan uint64, avglen))
		}

		// local addr index never change
		for connidx, _ := range eng.laddrList {
			eng.laddrPool[connidx%eng.conf.cpus] <- uint64(connidx)
		}

		//
		if eng.conf.httpprofile != "" {
			httpProfile(eng.conf.httpprofile)
		}

		//
		if eng.conf.profile {

			eng.pprofbase = filepath.Dir(eng.binfile) + "/pprof.client." + strconv.Itoa(os.Getpid())

			eng.cpufile = eng.pprofbase + ".cpu.prof"
			pfd, err := os.OpenFile(eng.cpufile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
			if err != nil {
				fmt.Printf("OpenFile: %s\n", err.Error())
				os.Exit(1)
			}
			if err := pprof.StartCPUProfile(pfd); err != nil {
				fmt.Printf("StartCPUProfile: %s\n", err.Error())
				os.Exit(1)
			}

			//
			eng.heapstart = eng.pprofbase + ".heap.start.prof"
			eng.heapend = eng.pprofbase + ".heap.end.prof"

			//
			heappfd, err := os.OpenFile(eng.heapstart, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
			if err != nil {
				fmt.Printf("OpenFile: %s\n", err.Error())
				os.Exit(1)
			}
			if err := pprof.WriteHeapProfile(heappfd); err != nil {
				fmt.Printf("WriteHeapProfile: %s\n", err.Error())
				os.Exit(1)
			}

			fmt.Printf("\n -- profile enabled, https://blog.golang.org/profiling-go-programs\n")
			fmt.Printf("\n go tool pprof %s %s\n", eng.binfile, eng.cpufile)
			fmt.Printf("\n go tool pprof %s %s\n", eng.binfile, eng.heapstart)
			fmt.Printf("\n go tool pprof %s %s\n", eng.binfile, eng.heapend)
			fmt.Printf("\n")
		}

		for idx = 0; idx < eng.conf.cpus; idx++ {
			eng.wg.Add(1)
			go eng.connector(idx, destAddr)
		}

	} else {
		// listener/server side

		if eng.conf.cpus == 0 {
			eng.conf.cpus = runtime.GOMAXPROCS(-1)/4 + 1
		}
		runtime.GOMAXPROCS(eng.conf.cpus)
		eng.conf.cpus = runtime.GOMAXPROCS(-1)

		fmt.Printf("[%d] %d worker listening at %s, max concurrent %d, max RW QPS %d, press <CTL-C> to exit ...\n", os.Getpid(), eng.conf.cpus, destAddr.String(), eng.conf.maxConcurrent, eng.conf.QPSLimit)

		l, err := net.ListenTCP("tcp", destAddr)
		if err != nil {
			fmt.Printf("%s\n", err.Error())
			return err
		}

		if eng.conf.httpprofile != "" {
			httpProfile(eng.conf.httpprofile)
		}
		//
		if eng.conf.profile {

			eng.pprofbase = filepath.Dir(eng.binfile) + "/pprof.server." + strconv.Itoa(os.Getpid())

			eng.cpufile = eng.pprofbase + ".cpu.prof"
			cpupfd, err := os.OpenFile(eng.cpufile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
			if err != nil {
				fmt.Printf("OpenFile: %s\n", err.Error())
				os.Exit(1)
			}
			if err := pprof.StartCPUProfile(cpupfd); err != nil {
				fmt.Printf("StartCPUProfile: %s\n", err.Error())
				os.Exit(1)
			}

			//
			eng.heapstart = eng.pprofbase + ".heap.start.prof"
			eng.heapend = eng.pprofbase + ".heap.end.prof"

			heappfd, err := os.OpenFile(eng.heapstart, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
			if err != nil {
				fmt.Printf("OpenFile: %s\n", err.Error())
				os.Exit(1)
			}
			if err := pprof.WriteHeapProfile(heappfd); err != nil {
				fmt.Printf("WriteHeapProfile: %s\n", err.Error())
				os.Exit(1)
			}

			fmt.Printf("\n -- profile enabled, https://blog.golang.org/profiling-go-programs\n")
			fmt.Printf("\n go tool pprof %s %s\n", eng.binfile, eng.cpufile)
			fmt.Printf("\n go tool pprof %s %s\n", eng.binfile, eng.heapstart)
			fmt.Printf("\n go tool pprof %s %s\n", eng.binfile, eng.heapend)
			fmt.Printf("\n")
		}

		eng.wg.Add(1)
		go eng.acceptor(idx, l)
		eng.listenfds = append(eng.listenfds, l)
		idx++
		var warned bool
		if eng.conf.monoMode {
			warned = true
			fmt.Printf("using the same listen fd for all acceptor.\n")
		}
		for ; idx < eng.conf.cpus; idx++ {
			eng.wg.Add(1)
			nl, err := net.ListenTCP("tcp", destAddr)
			if err != nil {
				if warned == false {
					warned = true
					fmt.Printf("no more listen fd, maybe need linux kernel >= 3.19: %s\n", err.Error())
					fmt.Printf("using the same listen fd for all acceptor.\n")
				}
				nl = l
			}
			eng.listenfds = append(eng.listenfds, nl)
			go eng.acceptor(idx, nl)
		}
	}

	//
	if eng.conf.maxConcurrent > 0 {
		go eng.maxUnlock(1)
	} else {
		close(eng.workswitch)
		eng.workloadon = true
	}
	if eng.conf.QPSLimit > 0 {
		// * 2 for no blocking
		eng.qpschannel = make(chan struct{}, eng.conf.QPSLimit*2)
		go eng.qpsctl()
	}
	go eng.gcRunner(10)
	if eng.conf.interval > 0 {
		go eng.stat()
	} else {
		fmt.Printf("stat show disabled.\n")
	}

	go eng.workerWait()

	if eng.conf.duration > 0 {
		fmt.Printf("\n -- will exit after %d seconds --\n", eng.conf.duration)
		go func() {
			time.Sleep(time.Duration(eng.conf.duration) * time.Second)
			fmt.Printf("\n -- auto close --\n")
			eng.Close()
		}()
	}

	return nil
}

// sigHandle
func (eng *Engine) sigHandle() {
	exitcnt := 0
	for {
		sig := <-eng.sigChan
		if eng.closing == false {
			notifyed := false
			for _, val := range eng.conf.notifys {
				if val == sig {
					fmt.Printf("\n -- got signal: %d/%s\n", sig, sig.String())
					notifyed = true
					break
				}
			}
			for _, val := range eng.conf.exits {
				if val == sig {
					fmt.Printf("\n -- exiting for signal: %d/%s\n", sig, sig.String())
					eng.closing = true

					//
					for _, l := range eng.listenfds {
						l.Close()
					}

					eng.maxCond.Broadcast()
					break
				}
			}
			if eng.closing == false && notifyed == false {
				fmt.Printf("\n -- unknow signal ignored: %d/%s\n", sig, sig.String())
			}
		} else {
			exitcnt++
		}
		if eng.closing && exitcnt > NET_FORCE_EXIT {
			fmt.Printf("\n -- exit directly for exit signal repeat %d times: %d/%s\n", exitcnt, sig, sig.String())
			os.Exit(1)
		}
	}
}

//

//
func (eng *Engine) workerWait() {
	eng.wg.Wait()

	eng.m.Lock()

	eng.closing = true

	//
	for _, l := range eng.listenfds {
		l.Close()
	}

	if eng.conf.profile {

		//
		heappfd, err := os.OpenFile(eng.heapend, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Printf("OpenFile: %s\n", err.Error())
		}
		if err := pprof.WriteHeapProfile(heappfd); err != nil {
			fmt.Printf("WriteHeapProfile: %s\n", err.Error())
		}
	}

	//
	eng.maxCond.Broadcast()

	//
	eng.maxLock.Lock()
	fmt.Printf("closing %d/%d exist connections ...\n", len(eng.conns), atomic.LoadInt64(eng.aliveCount))
	// without eng.conns, we can not close actived connections befor exit
	for _, conn := range eng.conns {
		conn.Close()
	}
	fmt.Printf("all connections closed.\n")
	select {
	case <-eng.workswitch:
	default:
		close(eng.workswitch)
	}
	eng.maxLock.Unlock()

	// wait for all handle exit
	tk := time.NewTicker(100 * time.Millisecond)
	defer tk.Stop()
	for {
		if atomic.LoadInt64(eng.aliveCount) == 0 {
			eng.closed = true
			fmt.Printf("all connections exited.\n")
			break
		}
		<-tk.C
	}

	if eng.conf.profile {
		pprof.StopCPUProfile()
	}

	close(eng.destroyed)
	fmt.Printf("engine closed.\n")
	eng.m.Unlock()
}

//
func (eng *Engine) Close() error {
	eng.m.Lock()
	defer eng.m.Unlock()
	if eng.closed {
		return errors.New("engine already closed")
	}
	//
	for _, l := range eng.listenfds {
		l.Close()
	}
	//
	eng.closing = true
	//
	return nil
}

//
func (eng *Engine) Wait() {
	<-eng.destroyed
}

//
func (eng *Engine) connector(idx int, destAddr *net.TCPAddr) {
	defer eng.wg.Done()
	//fmt.Printf("connector#%d ...\n", idx)
	var conn *net.TCPConn
	var err error
	var newconn bool
	var connidx uint64
	for {
		eng.maxLock.Lock()
		if eng.closing {
			eng.maxLock.Unlock()
			break
		}
		if eng.conf.maxConcurrent > 0 && atomic.LoadInt64(eng.aliveCount) >= eng.conf.maxConcurrent {
			if newconn {
				newconn = false
				conn.Close()
				conn = nil
				eng.laddrPool[idx] <- connidx
			}
			fmt.Printf("connector#%d hold on for reach max concurrent %d/%d\n", idx, atomic.LoadInt64(eng.aliveCount), eng.conf.maxConcurrent)
			if eng.conf.exitmax {
				eng.closing = true
				fmt.Printf("connector#%d, exited for reach max concurrent %d/%d.\n", idx, atomic.LoadInt64(eng.aliveCount), eng.conf.maxConcurrent)
				eng.maxLock.Unlock()
				break
			}
			if eng.workloadon == false {
				eng.workloadon = true
				// tell all handle to continue
				fmt.Printf("connector#%d work load on ...\n", idx)
				// delay for all connector sync
				time.Sleep(NET_MICRO_DELAY)
				// BUG: connect goroutine do not effect by close?
				close(eng.workswitch)
			}
			// Wait() = unlock + wait + lock
			eng.maxCond.Wait()
			eng.maxLock.Unlock()
			// double check
			continue
		}
		if newconn {
			newconn = false
			if eng.conf.nomap == false {
				eng.conns[connidx] = conn
			}
			atomic.AddInt64(eng.aliveCount, 1)
			go eng.clientHandle(idx, connidx, conn)
			//
		}
		//		if eng.closing {
		//			eng.maxLock.Unlock()
		//			break
		//		}
		eng.maxLock.Unlock()

		// use local addr index as map index
		connidx = <-eng.laddrPool[idx]

		// just connect
		conn, err = net.DialTCPTimeout("tcp", &eng.laddrList[connidx], destAddr, NET_CONNECT_TIMEOUT)
		if err == nil {
			newconn = true
			//
			atomic.AddInt64(eng.newCount, 1)
			continue
		}
		// TODO: better way to identify net error
		if strings.Index(err.Error(), "address already in use") != -1 {
			//time.Sleep(NET_MICRO_DELAY)
			continue
		}
		if strings.Index(err.Error(), "i/o timeout") != -1 {
			time.Sleep(NET_MICRO_DELAY)
			continue
		}
		//
		fmt.Printf("connector#%d exited for %s\n", idx, err.Error())

		eng.maxLock.Lock()
		eng.closing = true
		eng.maxLock.Unlock()

		break
	}
	eng.maxCond.Broadcast()
}

//
func (eng *Engine) acceptor(idx int, l *net.TCPListener) {
	defer eng.wg.Done()
	var conn *net.TCPConn
	var err error
	var newconn bool
	var connidx uint64
	//fmt.Printf("acceptor#%d ...\n", idx)
	for {
		eng.maxLock.Lock()
		if eng.closing {
			eng.maxLock.Unlock()
			break
		}
		if eng.conf.maxConcurrent > 0 && atomic.LoadInt64(eng.aliveCount) >= eng.conf.maxConcurrent {
			if newconn {
				newconn = false
				conn.Close()
				conn = nil
			}
			fmt.Printf("acceptor#%d hold on for reach max concurrent %d/%d\n", idx, atomic.LoadInt64(eng.aliveCount), eng.conf.maxConcurrent)
			if eng.conf.exitmax {
				eng.closing = true
				fmt.Printf("acceptor#%d, exited for reach max concurrent %d/%d.\n", idx, atomic.LoadInt64(eng.aliveCount), eng.conf.maxConcurrent)
				eng.maxLock.Unlock()
				break
			}
			if eng.workloadon == false {
				eng.workloadon = true
				// tell all handle to continue
				fmt.Printf("connector#%d work load on ...\n", idx)
				// delay for all acceptor sync
				time.Sleep(NET_MICRO_DELAY)
				// BUG: connect goroutine do not effect by close?
				close(eng.workswitch)
			}
			// Wait() = unlock + wait + lock
			eng.maxCond.Wait()
			eng.maxLock.Unlock()
			continue
		}
		if newconn {
			newconn = false
			if eng.conf.nomap == false {
				eng.conns[connidx] = conn
			}
			atomic.AddInt64(eng.aliveCount, 1)
			go eng.serverHandle(connidx, conn)
			//
		}
		//		if eng.closing {
		//			eng.maxLock.Unlock()
		//			break
		//		}
		eng.maxLock.Unlock()

		conn, err = l.AcceptTCP()
		if err == nil {
			newconn = true
			// use io counter as map index
			//
			connidx = uint64(atomic.AddInt64(eng.newCount, 1))
			continue
		}

		if eng.closing == false {
			fmt.Printf("acceptor#%d exited for %s\n", idx, err.Error())
			eng.maxLock.Lock()
			eng.closing = true
			eng.maxLock.Unlock()
		}
		break
	}
	//
	l.Close()
	eng.maxCond.Broadcast()
}

//
func (eng *Engine) maxUnlock(interval int) {
	if eng.conf.maxConcurrent == 0 {
		return
	}
	tk := time.NewTicker(time.Duration(interval) * time.Second)
	defer tk.Stop()
	for {
		if atomic.LoadInt64(eng.aliveCount) < eng.conf.maxConcurrent {
			eng.maxCond.Broadcast()
		}
		<-tk.C
	}
}

//
func (eng *Engine) serverHandle(connidx uint64, conn *net.TCPConn) {
	var blockEncrypt, blockDecrypt cipher.BlockMode

	// BUG: handle eat up CPUs, and main goroutine is hungry

	// waiting for start signal
	<-eng.workswitch

	aesblock, err := aes.NewCipher(eng.aeskey)
	if err != nil {
		panic(err.Error())
	}
	blockDecrypt = cipher.NewCBCDecrypter(aesblock, eng.ivbuf)

	if eng.conf.twoway {
		blockEncrypt = cipher.NewCBCEncrypter(aesblock, eng.ivbuf)
	}
	if eng.conf.noworkload {
		// read from peer until error
		// peer is reading too, so nothing happen
		for eng.closing == false {
			_, err = conn.Read(eng.dummyBuf)
			if err != nil {
				// when peer closed, got EOF.
				break
			}
			atomic.AddInt64(eng.rwCount, 1)
		}
	} else {
		// read from peer until error
		for eng.closing == false {
			if eng.conf.QPSLimit > 0 {
				<-eng.qpschannel
			}
			// work load

			// read request
			_, err = conn.Read(eng.dummyBuf)
			if err != nil {
				// when peer closed, got EOF.
				break
			}
			blockDecrypt.CryptBlocks(eng.dummyBuf, eng.dummyBuf)

			if eng.conf.twoway {
				blockEncrypt.CryptBlocks(eng.dummyBuf, eng.dummyBuf)
				// write response
				_, err = conn.Write(eng.dummyBuf)
				if err != nil {
					break
				}
			}
			atomic.AddInt64(eng.rwCount, 1)
		}
	}
	//
	conn.Close()

	conn = nil

	atomic.AddInt64(eng.aliveCount, -1)

	//
	if eng.conf.nomap == false {
		eng.maxLock.Lock()
		delete(eng.conns, connidx)
		eng.maxLock.Unlock()
	}
}

//
func (eng *Engine) clientHandle(idx int, connidx uint64, conn *net.TCPConn) {
	var blockEncrypt, blockDecrypt cipher.BlockMode

	// BUG: handle eat up CPUs, and main goroutine is hungry
	// should we impl goroutine-grouping

	// BUG: blockEncrypt only, qps = 3500000, blockEncrypt + blockDecrypt, qps = 280000, more then 10x decr.

	// waiting for start signal
	<-eng.workswitch

	aesblock, err := aes.NewCipher(eng.aeskey)
	if err != nil {
		panic(err.Error())
	}
	if eng.conf.twoway {
		blockDecrypt = cipher.NewCBCDecrypter(aesblock, eng.ivbuf)
	}
	blockEncrypt = cipher.NewCBCEncrypter(aesblock, eng.ivbuf)

	if eng.conf.noworkload {
		// read from peer until error
		// peer is reading too, so nothing happen
		for eng.closing == false {
			_, err = conn.Read(eng.dummyBuf)
			if err != nil {
				// when peer closed, got EOF.
				break
			}
			atomic.AddInt64(eng.rwCount, 1)
		}
	} else {
		// warite to peer until error
		for eng.closing == false {
			if eng.conf.QPSLimit > 0 {
				<-eng.qpschannel
			}
			// work load

			blockEncrypt.CryptBlocks(eng.dummyBuf, eng.dummyBuf)

			// write request
			_, err = conn.Write(eng.dummyBuf)
			if err != nil {
				break
			}
			if eng.conf.twoway {
				// read respose
				_, err = conn.Read(eng.dummyBuf)
				if err != nil {
					// when peer closed, got EOF.
					break
				}
				blockDecrypt.CryptBlocks(eng.dummyBuf, eng.dummyBuf)
			}
			atomic.AddInt64(eng.rwCount, 1)
		}
	}
	conn.Close()

	conn = nil

	atomic.AddInt64(eng.aliveCount, -1)

	eng.laddrPool[idx] <- connidx

	if eng.conf.nomap == false {
		eng.maxLock.Lock()
		delete(eng.conns, connidx)
		eng.maxLock.Unlock()
	}
}

//
func (eng *Engine) qpsctl() {
	// split to batch
	batch := 1
	qpsdelay := time.Microsecond * 1000
	var split int
	for {
		split = int(time.Second / qpsdelay)
		batch = eng.conf.QPSLimit / split
		if batch > 0 {
			break
		} else {
			qpsdelay += time.Millisecond
		}
	}
	if batch < 1 {
		batch = 1
	}
	if qpsdelay < 1 {
		qpsdelay = 1
	}

	fmt.Printf("QPS limit: delay %v, limit %v = %v * %v\n", qpsdelay, eng.conf.QPSLimit, int(split), batch)

	<-eng.workswitch

	tk := time.NewTicker(qpsdelay)
	defer tk.Stop()
	for {
		for idx := 0; idx < batch; idx++ {

			eng.qpschannel <- chanSignal

		}
		<-tk.C
	}

}

//
// for test only, run GC for profile GC in huge map
func (eng *Engine) gcRunner(interval int) {
	tk := time.NewTicker(time.Duration(interval) * time.Second)
	defer tk.Stop()
	var pre, cnt int64
	var iopre, iocnt int64
	var rwpre, rwcnt int64
	var norepeatgc bool
	for {
		//
		cnt = atomic.LoadInt64(eng.aliveCount)
		iocnt = atomic.LoadInt64(eng.newCount)
		rwcnt = atomic.LoadInt64(eng.rwCount)
		if cnt > 0 {
			if eng.conf.gctest && (pre != cnt || iopre != iocnt || rwpre != rwcnt) {
				fmt.Printf("GC with %d connections.\n", cnt)
				runtime.GC()
				//fmt.Printf("GC done.\n")
				pre = cnt
			}
			norepeatgc = false
		} else if norepeatgc == false {
			// even if eng.conf.gctest == false, still call FreeOSMemory when connections == 0
			norepeatgc = true
			fmt.Printf("FreeOSMemory with %d connections.\n", cnt)
			// free memory
			debug.FreeOSMemory()
			//fmt.Printf("FreeOSMemory done.\n")
			pre = cnt
		}
		<-tk.C
	}
}

//
func (eng *Engine) stat() {
	tk := time.NewTicker(time.Duration(eng.conf.interval) * time.Second)
	defer tk.Stop()
	var preTs = time.Now()

	var pre, cnt int64
	var iopre, iocnt int64
	var rwpre, rwcnt int64

	for {
		<-tk.C
		//
		cnt = atomic.LoadInt64(eng.aliveCount)
		iocnt = atomic.LoadInt64(eng.newCount)
		rwcnt = atomic.LoadInt64(eng.rwCount)
		if pre != cnt || iopre != iocnt || rwpre != rwcnt {
			//
			esp := time.Now().Sub(preTs)
			if esp <= 0 {
				esp = 1
			}
			qps := float64((cnt-pre)*int64(time.Second)) / float64(esp)
			ioqps := float64((iocnt-iopre)*int64(time.Second)) / float64(esp)
			rwqps := float64((rwcnt-rwpre)*int64(time.Second)) / float64(esp)
			fmt.Printf("concurrent %d/%d, esp %v, connection change %f/s, new connection %f/s, read-write %f/s.\n", cnt, eng.conf.maxConcurrent, esp, qps, ioqps, rwqps)
			pre = cnt
			iopre = iocnt
			rwpre = rwcnt
			preTs = time.Now()
		}
	}
}

//
func (eng *Engine) addressParse(laddrlist string) []net.TCPAddr {

	laddrList := make([]net.TCPAddr, 0, 1024)

	// parse Localaddr + Localport
	if len(laddrlist) == 0 {
		laddrlist = "0.0.0.0"
	}

	fmt.Printf("prepare local address: %s ...\n", laddrlist)

	localips := ResolveHostList(laddrlist)
	//fmt.Printf("localips(%d): %v\n", len(localips), localips)
	laddrcnt := int64(0)
	maxaddr := eng.conf.maxConcurrent * 2
	if eng.conf.maxConcurrent == 0 {
		maxaddr = math.MaxInt64
	}
	for _, ip := range localips {
		for lport := 8535; lport <= 65535; lport++ {
			laddr := ip + ":" + strconv.Itoa(lport)
			addr, err := net.ResolveTCPAddr("tcp", laddr)
			if err != nil {
				fmt.Printf("skipped: %s, %s\n", laddr, err.Error())
				continue
			}
			laddrList = append(laddrList, *addr)
			laddrcnt++
			if laddrcnt >= maxaddr {
				break
			}
		}
		if laddrcnt >= maxaddr {
			break
		}
	}

	laddrcnt = int64(len(laddrList))
	if eng.conf.maxConcurrent > laddrcnt {
		if eng.conf.maxConcurrent > 10000 {
			laddrcnt -= 500
		}
		fmt.Printf("WARNING: no enough local address+port, reduce max concurrent connection from %d to %d\n", eng.conf.maxConcurrent, laddrcnt)
		eng.conf.maxConcurrent = laddrcnt
	}
	return laddrList
}

//
// tcp Engine config
// command args
type Config struct {
	hostport      string
	showhelp      bool
	listenMode    bool
	monoMode      bool
	noworkload    bool
	exitmax       bool
	nomap         bool
	twoway        bool
	gctest        bool
	cpus          int
	QPSLimit      int
	duration      int
	interval      int
	maxConcurrent int64
	laddrlist     string
	profile       bool
	httpprofile   string
	notifys       []os.Signal // signal list for print only
	exits         []os.Signal // signal list for exit
}

// NewCmdConfig return Config base on command line args
func NewCmdConfig(notifys, exits []os.Signal) *Config {

	//

	conf := &Config{
		notifys: notifys,
		exits:   exits,
	}

	// read config from command line

	flag.BoolVar(&conf.showhelp, "h", false, "show help")

	flag.StringVar(&conf.hostport, "T", "127.0.0.1:5180", "target host:port for listener/connector")

	flag.BoolVar(&conf.listenMode, "l", false, "listen to host:port")

	flag.BoolVar(&conf.monoMode, "F", false, "using only one listen fd for all acceptor in listen mode")

	flag.BoolVar(&conf.exitmax, "E", false, "exit when reach max concurrent connections")

	flag.IntVar(&conf.cpus, "t", 0, "concurrent threads, default 1/2 of avaible CPUs")

	flag.Int64Var(&conf.maxConcurrent, "c", 0, "max concurrent connections, zero for unlimited")

	// you should ip addr add 127.0.0.10/8 dev lo befor launch connector(do it for all 127.0.0.10-180 IPs),
	// if hostport IP is not in the same machine, you should using out going ip/netmask for outside connecting.
	// make sure firewall in local machine do not block connect for too many connections.
	// bash script to add ip(run by root): for oneip in `seq 10 180`; do ip addr add 127.0.0.$oneip/8 dev lo; done
	flag.StringVar(&conf.laddrlist, "i", "127.0.0.10-180", "local ip list for connector")

	flag.StringVar(&conf.httpprofile, "H", "", "host:port for http profile")

	// http profile host:port
	flag.BoolVar(&conf.profile, "P", false, "pprof")

	flag.BoolVar(&conf.noworkload, "N", false, "do not read/write/crypto at new connection, holding/reading connections")

	flag.BoolVar(&conf.twoway, "W", false, "use bidirectional workload, must enable both in server/client at the same time")

	flag.BoolVar(&conf.gctest, "G", false, "call runtime.GC() when running")

	flag.BoolVar(&conf.nomap, "M", false, "disable connection in map tracing(and can not close running connection when exit)")

	flag.IntVar(&conf.QPSLimit, "q", 0, "qps limit pre-connection, zero for unlimit")

	flag.IntVar(&conf.interval, "I", 0, "interval for show stat, zero for disabled")

	flag.IntVar(&conf.duration, "d", 0, "run N seconds and exit, zero for disabled")

	flag.Parse()

	if len(conf.hostport) == 0 || conf.showhelp {
		flag.PrintDefaults()
		os.Exit(1)
	}

	if conf.maxConcurrent < 0 {
		conf.maxConcurrent = 0
	}

	if conf.QPSLimit <= 0 {
		conf.QPSLimit = 0
	}

	return conf
}

// ---- utils

//
// 10.0.0.1,10.0.0.5 or 10.0.0.1-100,10.0.0.201,www.163.com
func ResolveHostList(list string) (iplist map[string]string) {
	iplist = make(map[string]string, 0)

	list = strings.Replace(list, ",", " ", -1)
	list = strings.Replace(list, "  ", " ", -1)
	addrs := strings.Split(list, " ")
	if len(addrs) == 0 {
		return
	}
	for _, val := range addrs {

		val = strings.ToLower(val)
		shptr := strings.Index(val, "://")
		if shptr != -1 {
			// cut off http:// or https://
			val = val[:shptr]
		}

		ptr := strings.Index(val, "-")
		if ptr == -1 {
			iplist[val] = val
			continue
		}
		// found -
		tail := val[ptr:]
		val = val[:ptr]
		iplist[val] = val
		dotptr := strings.LastIndex(val, ".")
		if dotptr == -1 {
			// dot no found
			continue
		}
		if dotptr+1 > len(val) {
			// no end
			continue
		}
		firstint, err := strconv.Atoi(val[dotptr+1:])
		if err != nil {
			// no int
			continue
		}
		ptr = strings.Index(tail, "-")
		if ptr+1 > len(tail) {
			// no end
			continue
		}
		endint, err := strconv.Atoi(tail[ptr+1:])
		if err != nil {
			// no int end
			continue
		}
		if endint < firstint {
			pr := firstint
			firstint = endint
			endint = pr
		}
		firstint++
		for idx := firstint; idx <= endint; idx++ {
			oneip := val[:dotptr] + "." + strconv.Itoa(idx)
			iplist[oneip] = oneip
		}
	}
	//fmt.Printf("ResolveHostList(%d): %v\n", len(iplist), iplist)
	return
}

//
// 10.0.0.1:80,10.0.0.5:90 or 10.0.0.1-100:8080,10.0.0.201:9000,www.163.com:9986
func ResolveHostPortList(list string) (iplist map[string]string) {
	iplist = make(map[string]string)

	list = strings.Replace(list, ",", " ", -1)
	list = strings.Replace(list, "  ", " ", -1)
	addrs := strings.Split(list, " ")
	if len(addrs) == 0 {
		return
	}
	for _, val := range addrs {

		val = strings.ToLower(val)
		shptr := strings.Index(val, "://")
		if shptr != -1 {
			// cut off http:// or https://
			val = val[:shptr]
		}

		ptr := strings.LastIndex(val, ":")
		if ptr == -1 {
			// default port 80
			val = val + ":80"
			ptr = strings.LastIndex(val, ":")
		} else if ptr == len(val)-1 {
			val = val + "80"
			ptr = strings.LastIndex(val, ":")
		}
		// found -
		tail := val[ptr:]
		//fmt.Printf("ResolveHostPortList, port %s\n", tail)
		_, err := strconv.Atoi(tail[1:])
		if err != nil {
			// invalid port
			//fmt.Printf("ResolveHostPortList, port %s, %s\n", tail, err.Error())
			continue
		}
		val = val[:ptr]
		if len(val) == 0 {
			val = "0.0.0.0"
		}
		hlist := ResolveHostList(val)
		for _, val := range hlist {
			iplist[val+tail] = val + tail
		}
	}
	//fmt.Printf("ResolveHostPortList(%d): %v\n", len(iplist), iplist)
	return
}

//
//
//
//
