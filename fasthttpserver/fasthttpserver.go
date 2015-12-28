// fast http echo server for networking debug

package main

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"time"

	"net/http"
	_ "net/http/pprof"
	//"runtime/pprof"

	"github.com/wheelcomplex/preinit/misc"
)

const (
	HTTP_MAX_HEADER_SIZE      int = 128 // ab request = 106 bytes
	HTTP_INIT_HEADER_SIZE     int = 48  // wrk request = 40 bytes
	HTTP_MINI_HEADER_SIZE     int = 6   // GET /\n
	HTTP_MAX_HEADER_READ_SIZE int = 128 * 1024
	HTTP_TAIL_HEADER_SIZE     int = 4 // \r\n\r\n
)

// http state
const (
	HTTP_STATE_KEEPALIVE int = iota
	HTTP_STATE_READREQMETHOD
	HTTP_STATE_READREQHEADER
	HTTP_STATE_READREQDONE
	HTTP_STATE_PARSEREQHEADER
	HTTP_STATE_READREQBODY
	HTTP_STATE_PARSEREQBODY
	HTTP_STATE_SENDHEADER
	HTTP_STATE_SENDBODY
	HTTP_STATE_READ_CLOSED
	HTTP_STATE_WRITE_CLOSED
	HTTP_STATE_KEEPALIVE_CLOSED
)

// http method in int
type HttpIntMethodT int

const (
	HTTP_METHOD_UNSET HttpIntMethodT = iota
	HTTP_METHOD_GET
	HTTP_METHOD_HEAD
	HTTP_METHOD_POST
	HTTP_METHOD_NEEDMORE
	HTTP_METHOD_NOALLOW // 405 Method Not Allowed
	HTTP_METHOD_INVALID // 400 Bad Request
)

// An array isn't immutable by nature, you can't make it constant
var http200keepalive []byte = []byte("HTTP/1.0 200 OK\r\nContent-Length: 0\r\nConnection: keep-alive\r\n\r\n")
var http200close []byte = []byte("HTTP/1.0 200 OK\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")

// An array isn't immutable by nature, you can't make it constant
var HTTP_BYTES_METHOD_GET []byte = []byte("GET /")
var HTTP_BYTES_METHOD_HEAD []byte = []byte("HEAD /")
var HTTP_BYTES_METHOD_POST []byte = []byte("POST /")
var HTTP_BYTES_HTTP11 []byte = []byte("http/1.1")
var HTTP_BYTES_CONNECTION_CLOSE []byte = []byte("connection: close")
var HTTP_BYTES_CONNECTION_KEEPALIVE []byte = []byte("connection: keep-alive")

const (
	HTTP_BYTE_CHAR_RT byte = byte('\n')
	HTTP_BYTE_CHAR_LF byte = byte('\r')
	HTTP_CHAR_RT           = '\n'
	HTTP_CHAR_LF           = '\r'
)

const MAX_EMPTY_IO int = 2

//
var tpln = misc.Tpln
var tpf = misc.Tpf

// configure
type configT struct {
	threads        int           //
	workers        int           //
	listenAddr     string        //
	batchAcceptLen int           // max connection in batch
	batchTimeout   time.Duration // accept timeout
	statsInterval  time.Duration //
	ioTimtout      time.Duration //
}

//
var config = configT{
	threads:        runtime.NumCPU() * 2,
	workers:        runtime.NumCPU(),
	listenAddr:     "0.0.0.0:8080",
	batchAcceptLen: 64,
	batchTimeout:   time.Millisecond * 50,
	statsInterval:  time.Second * 5,
	ioTimtout:      time.Millisecond * 100,
}

// server struct
type serverVarT struct {
	signalInCh    chan os.Signal // catch signal
	lastSignal    os.Signal      //
	exitSignaled  bool           //
	exitSignalDup int            //
	acceptedCh    chan int       //
	closedCh      chan int       //
	requestsCh    chan int       //
}

//
var serverVar = serverVarT{
	signalInCh:   make(chan os.Signal, 16),
	exitSignaled: false,
	acceptedCh:   make(chan int, 4096),
	closedCh:     make(chan int, 4096),
	requestsCh:   make(chan int, 4096),
}

//
func main() {
	misc.SetGoMaxCPUs(config.threads)

	laddr, lerr := net.ResolveTCPAddr("tcp4", config.listenAddr)
	if lerr != nil {
		tpln("ERROR: ResolveTCPAddr %s: %s", config.listenAddr, lerr)
		os.Exit(1)
	}
	var lfd *net.TCPListener
	for i := 0; i < 10; i++ {
		lfd, lerr = net.ListenTCP("tcp4", laddr)
		if lerr != nil {
			tpln("ERROR#%d: %s", i, lerr)
			if strings.HasSuffix(lerr.Error(), "bind: address already in use") {
				time.Sleep(5e8)
			} else {
				break
			}
		} else {
			break
		}
	}
	if lfd == nil {
		tpln("Exit for %s", lerr)
		os.Exit(1)
	}

	//profile for debug
	profileport := "6060"
	profileurl := "localhost:" + profileport
	tpf("profile: [http://localhost:%s/debug/pprof/\n]http://localhost:%s/debug/pprof/\n", profileport, profileport)
	go func() {
		// http://localhost:6060/debug/pprof/
		fmt.Println(http.ListenAndServe(profileurl, nil))
	}()

	//	f, err := os.Create("fasthttpserver.pprof")
	//	if err != nil {
	//		log.Fatal(err)
	//	}
	//	pprof.StartCPUProfile(f)
	//	// http://blog.golang.org/profiling-go-programs
	//	// go tool pprof fasthttpserver fasthttpserver.pprof
	//	defer pprof.StopCPUProfile()

	// launch request counter
	statswg := sync.WaitGroup{}
	statswg.Add(1)
	go requestCounter(&statswg)

	// exit notify
	signal.Notify(serverVar.signalInCh)
	go func() {
		for serverVar.lastSignal = range serverVar.signalInCh {
			if serverVar.exitSignaled == false {
				serverVar.exitSignaled = true
				tpln("all, exiting by notify signal %d", serverVar.lastSignal)
				// close listener and worker goroutine will exit
				lfd.Close()
			} else {
				serverVar.exitSignalDup++
				if serverVar.exitSignalDup < 10 {
					tpln("signal %d, exiting, please wait ...", serverVar.lastSignal)
				} else {
					tpln("signal %d * %d, aborted.", serverVar.lastSignal, serverVar.exitSignalDup)
					os.Exit(9)
				}
			}
		}
	}()

	// launch http worker
	workerwg := sync.WaitGroup{}
	//config.workers = 1
	for i := 0; i < config.workers; i++ {
		workerwg.Add(1)
		go worker(i, lfd, &workerwg)
	}
	tpln("%d workers, using %d threads, listening %s ...", config.workers, runtime.GOMAXPROCS(-1), config.listenAddr)
	// waiting for worker
	workerwg.Wait()
	close(serverVar.acceptedCh)
	close(serverVar.requestsCh)
	close(serverVar.closedCh)
	// close listener if no closed
	if serverVar.exitSignaled == false {
		lfd.Close()
	}
	//
	// waiting for stats
	statswg.Wait()
	tpln("All done!")
	signal.Reset()
	close(serverVar.signalInCh)
}

// setListenerDeadline used for debug
func setListenerDeadline(lfd *net.TCPListener, timeout time.Duration) error {
	return lfd.SetDeadline(time.Now().Add(timeout))
}

// worker accept new connection from listener, start service goroutine in batch
func worker(id int, lfd *net.TCPListener, wg *sync.WaitGroup) {
	tk := time.NewTicker(config.batchTimeout)
	closewg := sync.WaitGroup{}
	defer tk.Stop()
	defer wg.Done()
	defer closewg.Wait()

	// initial
	methodBufPool := sync.Pool{
		New: func() interface{} {
			// only HTTP_INIT_HEADER_SIZE byte, for CC attacker friendly
			return make([]byte, HTTP_INIT_HEADER_SIZE, HTTP_INIT_HEADER_SIZE)
		},
	}
	headerBufPool := sync.Pool{
		New: func() interface{} {
			// only HTTP_MAX_HEADER_SIZE byte for one http request header, all body discarded
			return make([]byte, HTTP_MAX_HEADER_SIZE, HTTP_MAX_HEADER_SIZE)
		},
	}
	connBatch := make([]*net.TCPConn, 0, config.batchAcceptLen)
	onload := false
	setListenerDeadline(lfd, config.batchTimeout)
	for serverVar.exitSignaled == false {
		// batch onload
		if len(connBatch) > 0 {
			if len(connBatch) >= config.batchAcceptLen {
				// batch is full
				//tpln("accept batch out for full: %d", len(connBatch))
				onload = true
			} else {
				// flush timeout
				select {
				case <-tk.C:
					//tpln("accept batch out for timeout(%v): %d", config.batchTimeout, len(connBatch))
					onload = true
				default:
				}
			}
			if onload {
				// another worker will get new connection
				closewg.Add(len(connBatch))
				for _, connfd := range connBatch {
					go tinyhttpservice(connfd, &methodBufPool, &headerBufPool, &closewg)
				}
				serverVar.acceptedCh <- len(connBatch)
				onload = false
				connBatch = connBatch[:0]
			}
		}
		//
		nfd, err := lfd.AcceptTCP()
		if err != nil {
			if timeouterr, ok := err.(net.Error); ok {
				if timeouterr.Timeout() {
					setListenerDeadline(lfd, config.batchTimeout)
					//tpln("accept %v", timeouterr)
					continue
				} else if timeouterr.Temporary() {
					setListenerDeadline(lfd, config.batchTimeout)
					//tpln("accept %v", timeouterr)
					continue
				}
			}
			//tpln("worker#%d, exit by error: %s", id, err.Error())
			break
		}
		connBatch = append(connBatch, nfd)
		//tpln("#%d, accepted: %v", id, nfd)
	}
	//tpln("worker#%d, exited", id)
}

// setConnDeadline used for debug
func setConnDeadline(rw *net.TCPConn, timeoutTs time.Time) error {
	return rw.SetDeadline(timeoutTs)
}

// tinyhttpservice accept http request and response fixed http header
// will exit when connection done or exit signal recieved
func tinyhttpservice(rw *net.TCPConn, methodBufPool, headerBufPool *sync.Pool, wg *sync.WaitGroup) {
	//rw.SetLinger(0)
	//bufrw := bufio.NewReadWriter(bufio.NewReader(rw), bufio.NewWriter(rw))
	bufrw := rw
	state := HTTP_STATE_KEEPALIVE
	methodbuf := methodBufPool.Get().([]byte)

	var timelimit time.Time
	var iobytes, bufiobytes, requestbytes int
	var ioerror error
	var totalrequest int
	var reqoutcount int
	var emptyio int
	var methodok bool
	var isKeepAlive bool
	var outbuf []byte
	var iobuf, hdrbuf, cmpbuf []byte

	//rwidx := -1
	//tpf("tinyhttpservice, new connection#%d, %v\n", rwidx, rw)
	// run state machine
	var exitState bool
	for exitState == false && serverVar.exitSignaled == false {
		switch state {
		case HTTP_STATE_KEEPALIVE:
			//tpf("conn#%d, keepalive request#%d ...\n", rwidx, totalrequest)
			bufiobytes = 0
			emptyio = 0
			requestbytes = 0
			methodok = false
			iobuf = methodbuf
			if totalrequest == 0 {
				timelimit = time.Now().Add(config.ioTimtout)
			} else {
				if isKeepAlive == false {
					state = HTTP_STATE_KEEPALIVE_CLOSED
					continue
				}
				timelimit = time.Now().Add(config.ioTimtout * 4)
			}
			state = HTTP_STATE_READREQMETHOD
			setConnDeadline(rw, timelimit)
			fallthrough
		case HTTP_STATE_READREQMETHOD:
			fallthrough
		case HTTP_STATE_READREQHEADER:
			//tpln("befor method read, HEADER BUF(%d): %s", bufiobytes, buf[:bufiobytes])
			iobytes, ioerror = bufrw.Read(iobuf[bufiobytes:])
			if iobytes <= 0 {
				if ioerror != nil {
					//tpf("conn#%d, close when read method(%d): %v\n", rwidx, iobytes, ioerror)
					// read error
					state = HTTP_STATE_READ_CLOSED
					continue
				}
				if emptyio >= MAX_EMPTY_IO {
					//tpf("conn#%d, close when read method(%d) too many empty read: %v\n", rwidx, iobytes, ioerror)
					// too many empty read
					state = HTTP_STATE_READ_CLOSED
					continue

				}
				time.Sleep(time.Millisecond)
				// read more
				emptyio++
				continue
			}
			// got bytes
			bufiobytes += iobytes
			requestbytes += iobytes
			//tpln("after method read, iobytes %d, err %v, HEADER BUF(%d): %s", iobytes, ioerror, bufiobytes, iobuf[:bufiobytes])
			// try to parse
			if requestbytes < HTTP_MINI_HEADER_SIZE {
				if ioerror != nil {
					//tpf("conn#%d, close when read method(%d): %v\n", rwidx, iobytes, ioerror)
					// read error without valid method
					state = HTTP_STATE_READ_CLOSED
					continue
				}
				// read more
				continue
			}
			if methodok == false {
				httpIntMethod, _ := httpMethodParser(iobuf[:bufiobytes])
				if httpIntMethod > HTTP_METHOD_POST || httpIntMethod < HTTP_METHOD_GET {
					// invalid method line
					state = HTTP_STATE_READ_CLOSED
					continue
				}
				// first try
				// parse header
				pcode, _ := httpURIParser(iobuf[:bufiobytes])
				if pcode == 0 {
					// LFRT found in method buf, parse done
					state = HTTP_STATE_READREQDONE
					//tpln("LFRT found in methodbuf(%d): %s", bufiobytes, iobuf[:bufiobytes])
					continue
				}
				methodok = true
				if hdrbuf == nil {
					hdrbuf = headerBufPool.Get().([]byte)
				}
				copy(hdrbuf, methodbuf)
				iobuf = hdrbuf
			}
			// parse header
			pcode, _ := httpURIParser(iobuf[:bufiobytes])
			if pcode > 0 {
				if ioerror != nil {
					//tpf("conn#%d, close when header part read\n", rwidx)
					// read error without valid header
					state = HTTP_STATE_READ_CLOSED
					continue
				}
				// too many bytes read
				if requestbytes >= HTTP_MAX_HEADER_READ_SIZE {
					//tpf("conn#%d, close for LFRT no found(read %d >= %d).\n", rwidx, requestbytes, HTTP_MAX_HEADER_READ_SIZE)
					state = HTTP_STATE_READ_CLOSED
					continue
				}
				if bufiobytes >= HTTP_MAX_HEADER_SIZE {
					// HTTP_TAIL_HEADER_SIZE < HTTP_MINI_HEADER_SIZE
					// reuse buffer
					copy(iobuf, iobuf[bufiobytes-HTTP_TAIL_HEADER_SIZE:bufiobytes])
					bufiobytes = HTTP_TAIL_HEADER_SIZE
				}
				// read more
				//tpf("conn#%d, read more header for LFRT no found.\n", rwidx)
				state = HTTP_STATE_READREQHEADER
				continue
			}
			// it is ok, \n\n found
			state = HTTP_STATE_READREQDONE
			fallthrough
		case HTTP_STATE_READREQDONE:
			fallthrough
		case HTTP_STATE_PARSEREQHEADER:
			// it is ok, \n\n found
			totalrequest++
			reqoutcount++
			if reqoutcount >= 1000 {
				//tpln("requestsCh <- %d", reqoutcount)
				serverVar.requestsCh <- reqoutcount
				reqoutcount = 0
			}
			//tpf("conn#%d, LFRT found.\n", rwidx)
			if cmpbuf == nil {
				cmpbuf = headerBufPool.Get().([]byte)
			}
			isKeepAlive = httpKeepAliveParse(iobuf[:bufiobytes], cmpbuf)
			state = HTTP_STATE_READREQBODY
			fallthrough
		case HTTP_STATE_READREQBODY:
			// TODO: read/parse header Content-Length: 0
			fallthrough
		case HTTP_STATE_PARSEREQBODY:
			// initial for send response
			timelimit = time.Now().Add(config.ioTimtout)
			setConnDeadline(rw, timelimit)
			bufiobytes = 0
			emptyio = 0
			state = HTTP_STATE_SENDBODY
			outbuf = http200keepalive
			if isKeepAlive == false {
				outbuf = http200close
			}
			//tpln("response(%d): %s", len(outbuf), outbuf)
			// send outbuf response
			fallthrough
		case HTTP_STATE_SENDHEADER:
			fallthrough
		case HTTP_STATE_SENDBODY:
			iobytes, ioerror = bufrw.Write(outbuf[bufiobytes:])
			if iobytes > 0 {
				bufiobytes += iobytes
				if bufiobytes >= len(outbuf) {
					// keepalive supported, do not check keepalive header from client
					state = HTTP_STATE_KEEPALIVE
				}
				if ioerror != nil {
					// write error
					state = HTTP_STATE_WRITE_CLOSED
				}
				continue
			}
			if ioerror == nil {
				if emptyio < MAX_EMPTY_IO {
					// write more
					time.Sleep(time.Millisecond)
					emptyio++
				} else {
					// too many empty write
					state = HTTP_STATE_WRITE_CLOSED
				}
			} else {
				// write error
				state = HTTP_STATE_WRITE_CLOSED
			}
		case HTTP_STATE_READ_CLOSED:
			exitState = true
		case HTTP_STATE_WRITE_CLOSED:
			exitState = true
		case HTTP_STATE_KEEPALIVE_CLOSED:
			exitState = true
		default:
			panic(fmt.Sprintf("unknow http state machine %d", state))
		}
	}
	//	if serverVar.exitSignaled {
	//		tpln("conn#%d, closed for signal", rwidx)
	//	}
	serverVar.closedCh <- 1
	rw.Close()
	methodBufPool.Put(methodbuf)
	if hdrbuf != nil {
		headerBufPool.Put(hdrbuf)
	}
	if cmpbuf != nil {
		headerBufPool.Put(cmpbuf)
	}
	//tpln("requestsCh <- %d", reqoutcount)
	serverVar.requestsCh <- reqoutcount
	// do not use defer in high qps call
	wg.Done()
	//tpf("tinyhttpservice, exit after %d requests, %v\n", totalrequest, rw)
}

// httpMethodParser return http method of request
func httpMethodParser(buf []byte) (HttpIntMethodT, error) {
	//tpf("httpMethodParser, %d: %s\n", len(buf), buf)
	if len(buf) < HTTP_MINI_HEADER_SIZE {
		// invalid request
		return HTTP_METHOD_NEEDMORE, errors.New("invalid http request method , short buf")
	}
	if buf[0] != byte('G') && buf[0] != byte('P') && buf[0] != byte('H') {
		// invalid request
		return HTTP_METHOD_INVALID, errors.New("invalid http request method, ivalid char")
	}
	// len(buf) >= HTTP_MINI_HEADER_SIZE(6)
	if bytes.Equal(buf[:5], HTTP_BYTES_METHOD_GET) {
		// GET /
		//tpf("httpMethodParser, GET / %d: %s\n", len(buf), buf)
		return HTTP_METHOD_GET, nil
	}
	if bytes.Equal(buf[:6], HTTP_BYTES_METHOD_HEAD) {
		return HTTP_METHOD_HEAD, nil
	}
	if bytes.Equal(buf[:6], HTTP_BYTES_METHOD_POST) {
		return HTTP_METHOD_POST, nil
	}
	return HTTP_METHOD_NOALLOW, errors.New("no allow http request method")
}

// httpKeepAliveParse
func httpKeepAliveParse(buf, cmpbuf []byte) bool {
	var isKeepAlive bool
	var rbuf []byte
	if len(buf) > len(cmpbuf) {
		rbuf = cmpbuf
	} else {
		rbuf = buf
	}
	// to lower
	for idx, _ := range rbuf {
		if buf[idx] <= 'Z' && buf[idx] >= 'A' {
			// to lower
			cmpbuf[idx] = buf[idx] + 32
		} else {
			cmpbuf[idx] = buf[idx]
		}
	}
	// check version, http 1.1 default is keepalive
	ptr := bytes.Index(cmpbuf[:len(rbuf)], HTTP_BYTES_HTTP11)
	if ptr == -1 {
		isKeepAlive = false
	} else {
		isKeepAlive = true
	}
	// check connection header
	ptr = bytes.Index(cmpbuf[:len(rbuf)], HTTP_BYTES_CONNECTION_CLOSE)
	if ptr >= 0 {
		//tpln("\n%s, \nisKeepAlive %v", cmpbuf[:len(rbuf)], false)
		return false
	}
	ptr = bytes.Index(cmpbuf[:len(rbuf)], HTTP_BYTES_CONNECTION_KEEPALIVE)
	if ptr >= 0 {
		//tpln("\n%s, \nisKeepAlive %v", cmpbuf[:len(rbuf)], true)
		return true
	}
	//tpln("\n%s, \nisKeepAlive %v", cmpbuf[:len(rbuf)], isKeepAlive)
	//	tpln("%d, %d, %d", 'a', 'A', 'a'-'A')
	//	tpln("%d, %d, %d", 'z', 'Z', 'z'-'Z')
	return isKeepAlive
}

// httpURIParser find \r\n\r\n or \n\n
// return 0 for found, 1 for read more, 2 for error
func httpURIParser(buf []byte) (int, error) {
	//tpf("httpURIParser: %s\n", buf)
	//tpf("httpURIParser: %v\n", buf)
	for {
		buflen := len(buf)
		ptr := bytes.IndexByte(buf, HTTP_BYTE_CHAR_RT)
		//tpf("httpURIParser: %d, %v\n", ptr, ptr)
		if ptr < 0 {
			// read more
			return 1, nil
		} else {
			// ptr >= 0
			if ptr < HTTP_MINI_HEADER_SIZE {
				// invalid request
				return 2, errors.New("invalid http request when <n> found")
			}
			if ptr+1 >= buflen {
				// read more
				return 1, nil
			}
			if buf[ptr+1] == HTTP_CHAR_RT {
				// \n\n found
				return 0, nil
			}
			if ptr+2 >= buflen {
				// read more
				return 1, nil
			}
			if buf[ptr-1] == HTTP_CHAR_LF && buf[ptr+1] == HTTP_CHAR_LF && buf[ptr+2] == HTTP_CHAR_RT {
				// \r\n\r\n found
				return 0, nil
			}
			ptr++
			if ptr >= buflen {
				// read more
				return 1, nil
			}
			buf = buf[ptr:]
			// next check
		}
	}
}

// requestCounter
func requestCounter(wg *sync.WaitGroup) {
	defer wg.Done()
	tk := time.NewTicker(config.statsInterval)
	defer tk.Stop()

	var finishedNum, preFinishedNum, acceptNum, preAcceptNum, closedNum int
	var changed bool
	var exitCount int
	accts := time.Now()
	startts := time.Now()
	for exitCount < 3 {
		select {
		case num, ok := <-serverVar.requestsCh:
			if ok == false {
				exitCount++
				serverVar.requestsCh = nil
				continue
			}
			//tpln("%d <- requestsCh", num)
			finishedNum += num
			changed = true
		case num, ok := <-serverVar.acceptedCh:
			if ok == false {
				exitCount++
				serverVar.acceptedCh = nil
				continue
			}
			acceptNum += num
			changed = true
		case num, ok := <-serverVar.closedCh:
			if ok == false {
				exitCount++
				serverVar.closedCh = nil
				continue
			}
			closedNum += num
			changed = true
		case <-tk.C:
			if changed {
				esp := time.Now().Sub(accts)
				if esp < 1 {
					esp = 1
				}
				tpln("time esp %v, requests %d(qps: %d), accepted %d(qps: %d), closed %d, running %d", time.Now().Sub(startts), finishedNum, int(finishedNum-preFinishedNum)*int(time.Second)/int(esp), acceptNum, int(acceptNum-preAcceptNum)*int(time.Second)/int(esp), closedNum, acceptNum-closedNum)
			}
			changed = false
			preAcceptNum = acceptNum
			preFinishedNum = finishedNum
			accts = time.Now()
		}
	}
	esp := time.Now().Sub(accts)
	if esp < 1 {
		esp = 1
	}
	tpln("time esp %v, requests %d(qps: %d), accepted %d(qps: %d), closed %d, running %d", time.Now().Sub(startts), finishedNum, int(finishedNum-preFinishedNum)*int(time.Second)/int(esp), acceptNum, int(acceptNum-preAcceptNum)*int(time.Second)/int(esp), closedNum, acceptNum-closedNum)
}

//

//
