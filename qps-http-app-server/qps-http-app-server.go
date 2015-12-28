//
//http app server with qps counter.
//
//by wheelcomplex AT gmail com
//
//

package main

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/wheelcomplex/go-pkg-optargex"
)

const (
	//64kbytes
	RECV_BUF_LEN = 65536

	//HTTP Header date
	HttpTimeFormat = "Mon, 02 Jan 2006 15:04:05 GMT"
)

//command line arg
var srv_cfg_listen string = "0.0.0.0:8080"
var srv_cfg_max_connections int = 10240
var srv_cfg_use_cpu_num int = 0

var srv_cfg_net_timeout int = 15
var srv_cfg_net_keepalive_timeout int = 5

var srv_cfg_qps_interval int = 5

var srv_cfg_read_delay int = -1

var srv_cfg_write_delay int = -1

//short for http no keepalive, keepalive for http keepalive, readlonly for read but no response
var srv_cfg_mode string = "keepalive"

var srv_cfg_show_help bool = false
var srv_cfg_show_version bool = false

var srv_cfg_debug_level int = 0

var srv_cfg_var_conn_counter int32 = 0

const AdminEmail = "wheelcomplex@gmail.com"

//HTTP status codes, defined in RFC 2616
const (
	httpStatusContinue           = 100
	httpStatusSwitchingProtocols = 101

	httpStatusOK                   = 200
	httpStatusCreated              = 201
	httpStatusAccepted             = 202
	httpStatusNonAuthoritativeInfo = 203
	httpStatusNoContent            = 204
	httpStatusResetContent         = 205
	httpStatusPartialContent       = 206

	httpStatusMultipleChoices   = 300
	httpStatusMovedPermanently  = 301
	httpStatusFound             = 302
	httpStatusSeeOther          = 303
	httpStatusNotModified       = 304
	httpStatusUseProxy          = 305
	httpStatusTemporaryRedirect = 307

	httpStatusBadRequest                   = 400
	httpStatusUnauthorized                 = 401
	httpStatusPaymentRequired              = 402
	httpStatusForbidden                    = 403
	httpStatusNotFound                     = 404
	httpStatusMethodNotAllowed             = 405
	httpStatusNotAcceptable                = 406
	httpStatusProxyAuthRequired            = 407
	httpStatusRequestTimeout               = 408
	httpStatusConflict                     = 409
	httpStatusGone                         = 410
	httpStatusLengthRequired               = 411
	httpStatusPreconditionFailed           = 412
	httpStatusRequestEntityTooLarge        = 413
	httpStatusRequestURITooLong            = 414
	httpStatusUnsupportedMediaType         = 415
	httpStatusRequestedRangeNotSatisfiable = 416
	httpStatusExpectationFailed            = 417
	httpStatusTeapot                       = 418
	//from nginx
	httpStatusClientDisconnected = 499

	httpStatusInternalServerError     = 500
	httpStatusNotImplemented          = 501
	httpStatusBadGateway              = 502
	httpStatusServiceUnavailable      = 503
	httpStatusGatewayTimeout          = 504
	httpStatusHTTPVersionNotSupported = 505
)

var httpStatusText = map[int]string{

	//HTTP/1.1 200 OK

	httpStatusContinue:           " 100 Continue",
	httpStatusSwitchingProtocols: " 101 Switching Protocols",

	httpStatusOK:                   " 200 OK",
	httpStatusCreated:              " 201 Created",
	httpStatusAccepted:             " 202 Accepted",
	httpStatusNonAuthoritativeInfo: " 203 Non Authoritative Info",
	httpStatusNoContent:            " 204 No Content",
	httpStatusResetContent:         " 205 Reset Content",
	httpStatusPartialContent:       " 206 Partial Content",

	httpStatusMultipleChoices:   " 300 Multiple Choices",
	httpStatusMovedPermanently:  " 301 Moved Permanently",
	httpStatusFound:             " 302 Found",
	httpStatusSeeOther:          " 303 See Other",
	httpStatusNotModified:       " 304 NotModified",
	httpStatusUseProxy:          " 305 Use Proxy",
	httpStatusTemporaryRedirect: " 307 Temporary Redirect",

	httpStatusBadRequest:                   " 400 Bad Request",
	httpStatusUnauthorized:                 " 401 Unauthorized",
	httpStatusPaymentRequired:              " 402 Payment Required",
	httpStatusForbidden:                    " 403 Forbidden",
	httpStatusNotFound:                     " 404 Not Found",
	httpStatusMethodNotAllowed:             " 405 Method NotAllowed",
	httpStatusNotAcceptable:                " 406 Not Acceptable",
	httpStatusProxyAuthRequired:            " 407 Proxy Auth Required",
	httpStatusRequestTimeout:               " 408 Request Timeout",
	httpStatusConflict:                     " 409 Conflict",
	httpStatusGone:                         " 410 Gone",
	httpStatusLengthRequired:               " 411 Length Required",
	httpStatusPreconditionFailed:           " 412 Precondition Failed",
	httpStatusRequestEntityTooLarge:        " 413 Request Entity Too Large",
	httpStatusRequestURITooLong:            " 414 Request URI Too Long",
	httpStatusUnsupportedMediaType:         " 415 Unsupported Media Type",
	httpStatusRequestedRangeNotSatisfiable: " 416 Requested Range Not Satisfiable",
	httpStatusExpectationFailed:            " 417 Expectation Failed",
	httpStatusTeapot:                       " 418 Teapot",
	httpStatusClientDisconnected:           " 499 Client Disconnected",

	httpStatusInternalServerError:     " 500 Internal Server Error",
	httpStatusNotImplemented:          " 501 Not Implemented",
	httpStatusBadGateway:              " 502 Bad Gateway",
	httpStatusServiceUnavailable:      " 503 Service Unavailable",
	httpStatusGatewayTimeout:          " 504 Gateway Timeout",
	httpStatusHTTPVersionNotSupported: " 505 HTTP Version Not Supported",
}

//event state list
const (
	EVENT_START = iota

	TOTAL_TIME_ESP
	TOTAL_CONN_MAX
	TOTAL_CONN_RUNNING

	TOTAL_CONN_IDLE

	TOTAL_CONN_INIT
	TOTAL_CONN_KEEPALIVE
	TOTAL_CONN_REQUEST
	TOTAL_CONN_RESPONSE
	TOTAL_CONN_CLOSED

	HttpRequestStateReadInit
	HttpRequestStateReadWaiting
	HttpRequestStateReadHeader
	HttpRequestStateReadBody
	HttpRequestStatePaseError
	HttpRequestStateReadHeaderError
	HttpRequestStateReadHeaderTimeOut
	HttpRequestStateReadBodyError
	HttpRequestStatePaseDone

	HttpRequestStateWriteInit
	HttpRequestStateWriteHeader
	HttpRequestStateWriteHeaderDone
	HttpRequestStateWriteHeaderError
	HttpRequestStateWriteHeaderTimeOut
	HttpRequestStateWriteWaiting
	HttpRequestStateWriteBody
	HttpRequestStateWriteBodyDone
	HttpRequestStateWriteBodyError
	HttpRequestStateWriteBodyTimeOut

	HttpRequestStateRawProxy

	HttpRequestStateRawProxyReadError
	HttpRequestStateRawProxyWriteError

	HttpRequestStatefinished
	HttpRequestStateKeepalive
	HttpRequestStateDestroyed
	HttpRequestStateInvalid

	HttpRequestStateReadTimeOut
	HttpRequestStateReadError
	HttpRequestStateReadDone

	HttpRequestStateWriteTimeOut
	HttpRequestStateWriteError
	HttpRequestStateWriteDone

	EVENT_LAST
)

//event state string
var EventChangeString = map[int]string{

	EVENT_START:     "event cont start",
	TOTAL_TIME_ESP:  "time esp secondss",
	TOTAL_CONN_MAX:  "maximum connections",
	TOTAL_CONN_IDLE: "idle connections",

	TOTAL_CONN_INIT:      "new connections",
	TOTAL_CONN_KEEPALIVE: "keepalive requests",

	TOTAL_CONN_RUNNING: "connections running",
	TOTAL_CONN_CLOSED:  "connections closed",

	TOTAL_CONN_REQUEST:  "total requests",
	TOTAL_CONN_RESPONSE: "total response",

	HttpRequestStateReadInit:          "http read init",
	HttpRequestStateReadWaiting:       "http read waiting",
	HttpRequestStateReadHeader:        "http read header",
	HttpRequestStateReadBody:          "http read body",
	HttpRequestStatePaseError:         "http parse error",
	HttpRequestStateReadHeaderError:   "http read header error",
	HttpRequestStateReadHeaderTimeOut: "http read header time out",
	HttpRequestStateReadBodyError:     "http read header error",
	HttpRequestStatePaseDone:          "http parse done ",

	HttpRequestStateWriteInit:          "http write init",
	HttpRequestStateWriteHeader:        "http write header",
	HttpRequestStateWriteHeaderDone:    "http write header done",
	HttpRequestStateWriteHeaderTimeOut: "http write header time out",
	HttpRequestStateWriteHeaderError:   "http write header error",
	HttpRequestStateWriteWaiting:       "http write waiting",
	HttpRequestStateWriteBody:          "http write body",
	HttpRequestStateWriteBodyDone:      "http write body done",
	HttpRequestStateWriteBodyError:     "http write body error",
	HttpRequestStateWriteBodyTimeOut:   "http write body timeout ",

	HttpRequestStateRawProxy:           "raw proxy",
	HttpRequestStateRawProxyReadError:  "raw proxy read error",
	HttpRequestStateRawProxyWriteError: "raw proxy write error",

	HttpRequestStateKeepalive: "http keepalive",
	HttpRequestStateDestroyed: "http destroyed",
	HttpRequestStateInvalid:   "http invalid",
	HttpRequestStatefinished:  "http finished",

	HttpRequestStateReadTimeOut: "http read timeout ",
	HttpRequestStateReadError:   "http read error ",
	HttpRequestStateReadDone:    "http read done ",

	HttpRequestStateWriteTimeOut: "http write timeout ",
	HttpRequestStateWriteError:   "http write error ",
	HttpRequestStateWriteDone:    "http write done ",

	EVENT_LAST: "event cont last",
}

//
func commandArgsParser() {
	//
	optargex.SetVersion("WheelPlatform HTTP QPS Server v1.0.0, Copyright @" + AdminEmail + ", 2012-")
	//
	optargex.Add("v", "version", "show version.", false)
	optargex.Add("h", "help", "show this help.", false)
	optargex.Add("l", "listen", "Listen Address(format: ip-address:port)", "0.0.0.0:8080")
	optargex.Add("c", "connections", "max connections.", 10240)
	optargex.Add("C", "cpus", "used cpu number(< 2 for auto).", 0)
	optargex.Add("d", "debug-level", "debug level", 0)
	optargex.Add("t", "timeout", "IO timeout, in seconds", 15)
	optargex.Add("k", "keepalive", "keepalive timeout, in seconds", 5)
	optargex.Add("R", "read-delay", "delay befor read request, in seconds", -1)
	optargex.Add("W", "write-delay", "delay befor write request, in seconds", -1)
	optargex.Add("Q", "qps-interval", "qps show interval, in seconds", 5)
	optargex.Add("m", "mode", "runing mode, short for http no keepalive, keepalive for http keepalive, readlonly for read but no response", "keepalive")

	// Parse os.Args
	for opt := range optargex.Parse() {
		//fmt.Fprintf(os.Stderr, "checking: %v, %v\n", opt.ShortName, opt.Name)
		switch opt.ShortName {
		case "v":
			srv_cfg_show_version = opt.Bool()
		case "h":
			srv_cfg_show_help = opt.Bool()
		case "l":
			srv_cfg_listen = opt.String()
		case "m":
			srv_cfg_mode = opt.String()
		case "c":
			srv_cfg_max_connections = opt.Int()
		case "C":
			srv_cfg_use_cpu_num = opt.Int()
		case "d":
			srv_cfg_debug_level = opt.Int()
		case "t":
			srv_cfg_net_timeout = opt.Int()
		case "k":
			srv_cfg_net_keepalive_timeout = opt.Int()
		case "R":
			srv_cfg_read_delay = opt.Int()
		case "W":
			srv_cfg_write_delay = opt.Int()
		case "q":
			srv_cfg_qps_interval = opt.Int()

		}
		switch opt.Name {
		case "version":
			srv_cfg_show_version = opt.Bool()
		case "help":
			srv_cfg_show_help = opt.Bool()
		case "listen":
			srv_cfg_listen = opt.String()
		case "mode":
			srv_cfg_mode = opt.String()
		case "connections":
			srv_cfg_max_connections = opt.Int()
		case "cpus":
			srv_cfg_use_cpu_num = opt.Int()
		case "debug-level":
			srv_cfg_debug_level = opt.Int()
		case "timeout":
			srv_cfg_net_timeout = opt.Int()
		case "keepalive":
			srv_cfg_net_keepalive_timeout = opt.Int()
		case "read-delay":
			srv_cfg_read_delay = opt.Int()
		case "write-delay":
			srv_cfg_write_delay = opt.Int()
		case "qps-interval":
			srv_cfg_qps_interval = opt.Int()
		}
	}
	if srv_cfg_debug_level < 0 {
		srv_cfg_debug_level = 0
	}
	if srv_cfg_net_keepalive_timeout < 1 {
		srv_cfg_net_keepalive_timeout = 1
	}
	if srv_cfg_net_timeout < 1 {
		srv_cfg_net_timeout = 1
	}
	if srv_cfg_max_connections < 1 {
		fmt.Fprintf(os.Stderr, "ERROR: invalid max-connections %d, should greate then zero.\r\n", srv_cfg_max_connections)
		optargex.Usage()
		os.Exit(1)
	}
	if srv_cfg_show_version {
		optargex.Version()
		os.Exit(1)
	}
	if srv_cfg_show_help {
		optargex.Usage()
		os.Exit(1)
	}

	if runtime.NumCPU() > 1 {
		/*
			if runtime.NumCPU() > 1 {
				runtime.GOMAXPROCS(runtime.NumCPU() - 1)
			}
		*/
		if runtime.NumCPU() == 2 {
			println("WARNING: more then two CPU to run will be better.")
		}
		switch {
		case srv_cfg_use_cpu_num < 0 || runtime.NumCPU() == 2:
			runtime.GOMAXPROCS(runtime.NumCPU())
		case srv_cfg_use_cpu_num < 2:
			runtime.GOMAXPROCS(runtime.NumCPU() - 1)
		case runtime.NumCPU() >= srv_cfg_use_cpu_num:
			runtime.GOMAXPROCS(srv_cfg_use_cpu_num)
		case runtime.NumCPU() < srv_cfg_use_cpu_num:
			runtime.GOMAXPROCS(runtime.NumCPU())
			fmt.Fprintf(os.Stderr, "WARNING: execpt %d CPU to run, but onely %d CPU present.\r\n", srv_cfg_use_cpu_num, runtime.NumCPU())
		}
	} else {
		println("WARNING: need more then one CPU to run.")
		//os.Exit(1)
	}

	srv_cfg_listen, _ = netListenStringFormat(srv_cfg_listen)
	if srv_cfg_listen == "" {
		os.Exit(1)
	}
}

func blockSeconds(delay int) {
	timer := time.NewTimer(time.Duration(delay) * time.Second)
	<-timer.C
	return
}

// Index of rightmost occurrence of b in s.
func last(s string, b byte) int {
	i := len(s)
	for i--; i >= 0; i-- {
		if s[i] == b {
			break
		}
	}
	return i
}

//slice compare, use bytes.Equal

//check for listen string format
func netListenStringFormat(listenstring string) (final string, err error) {
	var tmp_host, tmp_port string
	var tmp_int int

	orig_listen := listenstring
	tmp_brackets_pos := last(listenstring, ']')
	if tmp_brackets_pos < 0 {
		tmp_brackets_pos = 0
	}
	if !strings.Contains(listenstring[tmp_brackets_pos:], ":") {
		tmp_int, _ = strconv.Atoi(listenstring)
		if tmp_int > 0 {
			//port only
			listenstring = ":" + listenstring
		} else {
			//address only
			listenstring = listenstring + ":8080"
		}
	}

	tmp_host, tmp_port, err = net.SplitHostPort(listenstring)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Parse listen address %v(%v) failed: %v\r\n", orig_listen, listenstring, err)
		final = ""
	} else {
		final = tmp_host + ":" + tmp_port
	}
	return
}

//TODO: raw stream chunk buffer chain
//rawStreamChunk type define
type rawStreamChunk_t []byte

//httpHeader type define
type httpHeader_t struct {
	key   rawStreamChunk_t
	value rawStreamChunk_t

	//support for duplicate header
	next *httpHeader_t
}

//filterContext type define
type filterFunc_t func(httpConnectionInfo *httpConnectionInfo_t) (err error)

//filterContext type define
type filterContext_t struct {
	name       string
	priority   int
	filterFunc filterFunc_t
}

//filterContextList type define
type filterContextList map[string]filterContext_t

//filterContextData type define
type filterContextData_t map[int]string

//filterContextDataList type define
type filterContextDataList map[string]filterContextData_t

//httpConnectionInfo type define
type httpConnectionInfo_t struct {
	conn    net.Conn
	ioError error

	eventNotifyChannel chan int

	ReadBuffer         []byte
	ReadRawStream      []byte
	ReadProcotolStream []byte
	ReadHeaderStream   []byte
	ReadBodyStream     []byte

	method string
	uri    string
	host   string

	//map key is hashHexString of header key byte stream
	requestHeaderList map[string]httpHeader_t

	state int

	keepalive    bool
	requestCount int

	//
	filterList     map[int]filterContextList
	filterDataList map[int]filterContextDataList

	//for fast response
	responseStatusCode  int
	responseHeaderText  []byte
	responseBodyText    []byte
	responsetHeaderList map[string]httpHeader_t
}

//get md5 hash of a byte slice
func byteSliceHashHexString(s1 []byte) string {
	hashmd5 := md5.New()
	io.WriteString(hashmd5, string(s1))
	return hex.EncodeToString(hashmd5.Sum(nil))
}

//http request handle Goroutine
func httpConnectionSetState(httpConnectionInfo *httpConnectionInfo_t, newState int) (oldState int) {
	if httpConnectionInfo == nil {
		oldState = HttpRequestStateInvalid
		return
	}
	oldState = httpConnectionInfo.state
	httpConnectionInfo.eventNotifyChannel <- newState
	httpConnectionInfo.state = newState
	return
}

//http request handle Goroutine
func httpConnectionInit(initStatusCode int, bodyText []byte, conn net.Conn, eventNotifyChannel chan int, requestCount int) (httpConnectionInfo *httpConnectionInfo_t) {

	httpConnectionInfo = new(httpConnectionInfo_t)

	httpConnectionInfo.conn = conn
	httpConnectionInfo.eventNotifyChannel = eventNotifyChannel

	httpConnectionSetState(httpConnectionInfo, HttpRequestStateReadInit)

	httpConnectionInfo.responseBodyText = append(httpConnectionInfo.responseBodyText, bodyText...)

	httpConnectionInfo.responseStatusCode = initStatusCode

	httpConnectionInfo.requestCount = requestCount

	return
}

//http request handle Goroutine
func httpConnectionKeepalive(httpConnectionInfo *httpConnectionInfo_t) (keepalive bool) {
	keepalive = httpConnectionInfo.keepalive
	if keepalive == false {
		//say goog-bye to this connection
		httpConnectionInfo.conn.Close()
		httpConnectionSetState(httpConnectionInfo, HttpRequestStateDestroyed)
		httpConnectionInfo = nil
	}
	return
}

//http request handle Goroutine
func httpConnectionHandler(initStatusCode int, bodyText []byte, conn net.Conn, eventNotifyChannel chan int) {

	var httpConnectionInfo *httpConnectionInfo_t

	//direct response for httpStatusServiceUnavailable
	if initStatusCode == httpStatusServiceUnavailable {
		//
		httpConnectionInfo = httpConnectionInit(initStatusCode, bodyText, conn, eventNotifyChannel, 1)
		//
		httpReadRequest(httpConnectionInfo)
		//
		if httpConnectionInfo.state == HttpRequestStatePaseDone {
			httpSendResponse(httpConnectionInfo)
		}
		httpConnectionInfo.keepalive = false
		//
		httpConnectionKeepalive(httpConnectionInfo)
		return
	}

	//keepalive loop
	var requestCount int = 0
	for {

		//new request or keepalive
		//old httpConnectionInfo discarded
		requestCount++
		httpConnectionInfo = httpConnectionInit(HttpRequestStateReadInit, bodyText, conn, eventNotifyChannel, requestCount)

		//fmt.Printf("new/keepalive request#%v: %v\r\n", httpConnectionInfo.requestCount, EventChangeString[httpConnectionInfo.state])

		httpReadRequest(httpConnectionInfo)

		//HttpRequestStatePaseDone
		if httpConnectionInfo.state != HttpRequestStatePaseDone {
			//request read/parse error, nothing to do, close
			//fmt.Printf("request#%v: %v\r\n", httpConnectionInfo.requestCount, EventChangeString[httpConnectionInfo.state])
			httpConnectionInfo.keepalive = false
			httpConnectionKeepalive(httpConnectionInfo)
			return
		}

		if httpConnectionInfo.requestCount > 1 {
			httpConnectionInfo.eventNotifyChannel <- TOTAL_CONN_KEEPALIVE
		}

		httpConnectionInfo.responseStatusCode = httpStatusOK

		//fmt.Printf("request#%v: %v\r\n", httpConnectionInfo.requestCount, EventChangeString[httpConnectionInfo.state])

		//nothing to write for mode readlonly
		if srv_cfg_mode == "readlonly" {
			//no io time out for readlonly, read until ioError
			//reuse httpConnectionInfo.ReadRawStream
			var TotalReadBytes int
			var in_bytes int

			for {
				in_bytes = httpReadRawStream(httpConnectionInfo, 0)
				if httpConnectionInfo.state != HttpRequestStateReadDone {
					httpConnectionSetState(httpConnectionInfo, HttpRequestStateReadBodyError)
					return
				}
				TotalReadBytes = TotalReadBytes + in_bytes
				fmt.Printf("total %v bytes, new %v.\r\n", TotalReadBytes, in_bytes)
				//fmt.Printf("%s\r\n", httpConnectionInfo.ReadRawStream)
			}
		}
		httpSendResponse(httpConnectionInfo)

		if httpConnectionKeepalive(httpConnectionInfo) == false {
			break
		}
	}
	return
}

//http read raw stream from connection
func httpReadRawStream(httpConnectionInfo *httpConnectionInfo_t, ioTimeOut int) (in_bytes int) {

	//TODO: use raw stream chunk pool

	//will try to read 5 * time.Now().Add(time.Duration(ioTimeOut) * time.Second
	var io_try_count int = 0
	var err error
	in_bytes = 0

	httpConnectionInfo.ioError = nil
	for in_bytes == 0 && httpConnectionInfo.ioError == nil {
		//fmt.Printf("read round start, timeout %v, in_bytes %v bytes, tryCount %v.\r\n", ioTimeOut, in_bytes, io_try_count)
		//fmt.Printf("%s\r\n", httpConnectionInfo.ReadBuffer)
		if ioTimeOut > 0 {
			httpConnectionInfo.conn.SetReadDeadline(time.Now().Add(time.Duration(ioTimeOut) * time.Second))
		}
		io_try_count++
		in_bytes, err = httpConnectionInfo.conn.Read(httpConnectionInfo.ReadBuffer)
		if err != nil {
			//LEARN: check net error
			if nerr, ok := err.(net.Error); ok {
				//fmt.Printf("%s\r\n", err.Error())
				switch {
				case ioTimeOut > 0 && (nerr.Timeout() || nerr.Temporary()):
					//fmt.Printf("reading Temporary/TimeOut: \r\n")
					if io_try_count >= 5 && ioTimeOut > 0 {
						//println("Error reading, Temporary/Timeout:", err.Error())
						httpConnectionSetState(httpConnectionInfo, HttpRequestStateReadTimeOut)
						httpConnectionInfo.ioError = err
						return
					}
				default:
					//fmt.Printf("Error reading: %s/%v\r\n", nerr, nerr)
					httpConnectionSetState(httpConnectionInfo, HttpRequestStateReadError)
					httpConnectionInfo.ioError = err
					return
				}
			} else {
				//fmt.Printf("Error reading: %s/%v\r\n", err, err)
				httpConnectionSetState(httpConnectionInfo, HttpRequestStateReadError)
				httpConnectionInfo.ioError = err
				return
			}
		}
		//fmt.Printf("read round end, in_bytes %v bytes, tryCount %v.\r\n", in_bytes, io_try_count)
		//fmt.Printf("%s\r\n", httpConnectionInfo.ReadBuffer)
		//slicePosition int = 0
	}
	httpConnectionInfo.ReadRawStream = append(httpConnectionInfo.ReadRawStream, httpConnectionInfo.ReadBuffer[:in_bytes]...)
	if httpConnectionInfo.ioError == nil {
		httpConnectionSetState(httpConnectionInfo, HttpRequestStateReadDone)
	}
	//fmt.Printf("read round end, in_bytes %v bytes, tryCount %v.\r\n", in_bytes, io_try_count)
	//fmt.Printf("%s\r\n", httpConnectionInfo.ReadRawStream)
	return
}

//http read request Goroutine
func httpReadRequest(httpConnectionInfo *httpConnectionInfo_t) {
	/*
		read http request and send back dyn text:
		GET / HTTP/1.0
		Host: localhost

		----

		HTTP/1.1 200 OK
		Cache-Control: no-cache, must-revalidate
		Content-Type: text/plain; charset=UTF-8
		Expires: Fri, 01 Jan 1990 00: 00: 00 GMT
		Date: Fri, 11 Jan 2013 09:18:46 GMT
		Pragma: no-cache
		Server: Wheelcomplex-Go-Qps-Server v1.0
		Content-Length: $LEN

		Connection:keep-alive
		Connection: close

		Wheelcomplex-Go-Qps-Server v1.0, runtime mode <readlonly/short/keepalive>
	*/

	//fmt.Printf("request#%v: %v\r\n", httpConnectionInfo.requestCount, EventChangeString[httpConnectionInfo.state])

	httpConnectionSetState(httpConnectionInfo, HttpRequestStateReadInit)
	//read delay
	if srv_cfg_read_delay > 0 {
		//fmt.Printf("Delay %v second befor read header.\r\n", srv_cfg_write_delay)
		httpConnectionSetState(httpConnectionInfo, HttpRequestStateReadWaiting)
		//time.Sleep(time.Duration(srv_cfg_read_delay) * time.Second)
		blockSeconds(srv_cfg_read_delay)
		//<-time.Tick(time.Duration(srv_cfg_read_delay) * time.Second)
		//fmt.Printf("Delay %v second done, read header.\r\n", srv_cfg_write_delay)
	}

	httpConnectionSetState(httpConnectionInfo, HttpRequestStateReadHeader)

	var RawStreamSeekPos int = 0
	var http_method_ok bool = false
	var http_header_startPos int = -1

	var TotalReadBytes int = 0
	var io_try_count int = 0
	var in_bytes int = 0
	var readTimeOut int = 0

	httpConnectionInfo.ReadBuffer = make([]byte, RECV_BUF_LEN)
	httpConnectionInfo.ioError = nil
	if httpConnectionInfo.requestCount > 1 {
		readTimeOut = srv_cfg_net_keepalive_timeout
	} else {
		readTimeOut = srv_cfg_net_timeout
	}

	for httpConnectionInfo.ioError == nil && io_try_count < readTimeOut {
		//
		io_try_count++

		in_bytes = httpReadRawStream(httpConnectionInfo, 1)
		TotalReadBytes = TotalReadBytes + in_bytes
		switch httpConnectionInfo.state {
		case HttpRequestStateReadDone:
			httpConnectionSetState(httpConnectionInfo, HttpRequestStateReadHeader)
		case HttpRequestStateReadTimeOut:
			if TotalReadBytes < 16 && io_try_count == readTimeOut {
				httpConnectionSetState(httpConnectionInfo, HttpRequestStateReadHeaderTimeOut)
				return
			}
			httpConnectionSetState(httpConnectionInfo, HttpRequestStateReadHeader)
		default:
			//error
			if TotalReadBytes < 16 || io_try_count == readTimeOut {
				//fmt.Printf("read header error, TotalReadBytes %v, in_bytes %v bytes, tryCount %v, %v.\r\n", TotalReadBytes, in_bytes, io_try_count, httpConnectionInfo.ReadRawStream)
				httpConnectionSetState(httpConnectionInfo, HttpRequestStateReadHeaderError)
				return
			}
			httpConnectionSetState(httpConnectionInfo, HttpRequestStateReadHeader)
		}
		//fmt.Printf("read header done, TotalReadBytes %v, in_bytes %v bytes, tryCount %v, error %v, [%v].\r\n", TotalReadBytes, in_bytes, io_try_count, httpConnectionInfo.ioError, httpConnectionInfo.ReadRawStream)
		//checking for HTTP procotol
		//try to found "GET / HTTP/1.0\r\n" len = 16
		if http_method_ok == false {
			//last
			/*
				GET/POST/DELETE/PUT/HEAD / HTTP/1.0
				Host: localhost
			*/
			//seek for a \n
			returnPos := bytes.IndexByte(httpConnectionInfo.ReadRawStream[RawStreamSeekPos:], '\n')
			//fmt.Printf("finding first \\n for protocol line: start %d, found %d, %v\r\n", RawStreamSeekPos, returnPos, httpConnectionInfo.ReadRawStream[RawStreamSeekPos:])
			switch {
			case returnPos == -1:
				//no found
				//fmt.Printf("no found, finding first \\n for protocol line: start %d, found %d, %v\r\n", RawStreamSeekPos, returnPos, httpConnectionInfo.ReadRawStream[RawStreamSeekPos:])
			case returnPos == 0:
				//start from \n
				//fmt.Printf("start from \\n, finding first \\n for protocol line: start %d, found %d, %v\r\n", RawStreamSeekPos, returnPos, httpConnectionInfo.ReadRawStream[RawStreamSeekPos:])
				fallthrough
			case returnPos == 1 && httpConnectionInfo.ReadRawStream[RawStreamSeekPos] == '\r':
				//start from \r\n
				//fmt.Printf("start from \\r\\n, finding first \\n for protocol line: start %d, found %d, %v\r\n", RawStreamSeekPos, returnPos, httpConnectionInfo.ReadRawStream[RawStreamSeekPos:])
				if RawStreamSeekPos > 14 {
					//too many empty line, send httpStatusBadRequest
					httpConnectionInfo.state = HttpRequestStatePaseError
					httpConnectionInfo.responseStatusCode = httpStatusBadRequest
					httpSendResponse(httpConnectionInfo)
					return
				} else {
					//skip empty \n
					RawStreamSeekPos = RawStreamSeekPos + returnPos + 1
				}
			default:
				//found first \n
				if TotalReadBytes < 16 {
					//too few char in protocol line, send httpStatusBadRequest
					httpConnectionInfo.state = HttpRequestStatePaseError
					httpConnectionInfo.responseStatusCode = httpStatusBadRequest
					httpSendResponse(httpConnectionInfo)
					return
				}
				httpConnectionInfo.ReadProcotolStream = httpConnectionInfo.ReadRawStream[RawStreamSeekPos : RawStreamSeekPos+returnPos]
				//fmt.Printf("Found first \\n for protocol line: %s\r\n", string(httpConnectionInfo.ReadProcotolStream))
				switch {
				case bytes.HasPrefix(httpConnectionInfo.ReadRawStream[RawStreamSeekPos:], []byte("GET /")):
					http_method_ok = true
					httpConnectionInfo.method = "GET"
					//+4 for URI start
					//RawStreamSeekPos = RawStreamSeekPos + 4
					http_header_startPos = returnPos + 1
					RawStreamSeekPos = RawStreamSeekPos + returnPos + 1
				case bytes.HasPrefix(httpConnectionInfo.ReadRawStream[RawStreamSeekPos:], []byte("POST /")):
					http_method_ok = true
					httpConnectionInfo.method = "POST"
					//+5 for URI start
					//RawStreamSeekPos = RawStreamSeekPos + 5
					http_header_startPos = returnPos + 1
					RawStreamSeekPos = RawStreamSeekPos + returnPos + 1
					//
				default:
					//unknow method
					httpConnectionInfo.state = HttpRequestStatePaseError
					httpConnectionInfo.responseStatusCode = httpStatusMethodNotAllowed
					httpSendResponse(httpConnectionInfo)
					return
				}
			}
			//method no found, read more
		}

		//try to found header end \r\n\r\n or \n\n
		for http_method_ok && RawStreamSeekPos < TotalReadBytes && httpConnectionInfo.state != HttpRequestStatePaseDone {
			//seek for a \n
			returnPos := bytes.IndexByte(httpConnectionInfo.ReadRawStream[RawStreamSeekPos:], '\n')
			//fmt.Printf("finding first \\n for header ending line: start %d, found %d, prefix %v, %v\r\n", RawStreamSeekPos, returnPos, httpConnectionInfo.ReadRawStream[RawStreamSeekPos], httpConnectionInfo.ReadRawStream[RawStreamSeekPos:])
			switch {
			case returnPos == -1:
				//no found
			case returnPos == 0:
				//start from \n
				fallthrough
			case returnPos == 1 && httpConnectionInfo.ReadRawStream[RawStreamSeekPos] == '\r':
				//start from \r\n
				httpConnectionInfo.state = HttpRequestStatePaseDone
				httpConnectionInfo.ReadHeaderStream = httpConnectionInfo.ReadRawStream[http_header_startPos : RawStreamSeekPos+returnPos]
				if returnPos < len(httpConnectionInfo.ReadRawStream) {
					httpConnectionInfo.ReadBodyStream = httpConnectionInfo.ReadRawStream[RawStreamSeekPos+returnPos+1:]
				}
			default:
				//found header with \n
				//TODO: save header info
				//move forward
				RawStreamSeekPos = RawStreamSeekPos + returnPos + 1
			}
			//header no end, read more
		}
		if httpConnectionInfo.state == HttpRequestStatePaseDone {
			//fmt.Printf("HTTP Header end: header [%s], body [%s]\r\n", string(httpConnectionInfo.ReadHeaderStream), string(httpConnectionInfo.ReadBodyStream))
			httpConnectionInfo.eventNotifyChannel <- TOTAL_CONN_REQUEST
			break
		}
		//NOTICE: we do not parse http version/method/content-type/content-lenght
		//NOTICE: we do not read request body
	}

	//fmt.Printf("done reading, total %v bytes.\r\n", TotalReadBytes)
	//fmt.Printf("%s\r\n", httpConnectionInfo.ReadRawStream)

	return
}

//write byte stream to conn
func httpWriteResponser(httpConnectionInfo *httpConnectionInfo_t, text_body []byte, ioTimeOut int) {

	var write_total_bytes int = 0
	var write_round_bytes int = 0

	var text_len int
	var io_try_count int = 0
	var err error

	text_len = len(text_body)
	httpConnectionInfo.ioError = nil
	for httpConnectionInfo.ioError == nil {
		io_try_count++
		if ioTimeOut > 0 {
			httpConnectionInfo.conn.SetWriteDeadline(time.Now().Add(time.Duration(ioTimeOut) * time.Second))
		}
		write_round_bytes, err = httpConnectionInfo.conn.Write(text_body[write_total_bytes:])
		if err != nil {
			//LEARN: check net error
			if nerr, ok := err.(net.Error); ok {
				//fmt.Printf("%v\r\n", err)
				switch {
				case ioTimeOut > 0 && (nerr.Timeout() || nerr.Temporary()):
					//fmt.Printf("writing Temporary/TimeOut: \r\n")
					if io_try_count >= 5 && ioTimeOut > 0 {
						//println("Error writing, Temporary/Timeout:", err.Error())
						httpConnectionSetState(httpConnectionInfo, HttpRequestStateWriteTimeOut)
						httpConnectionInfo.ioError = err
					}
				default:
					//println("Error writing:", err.Error())
					httpConnectionSetState(httpConnectionInfo, HttpRequestStateWriteError)
					httpConnectionInfo.ioError = err
				}
			} else {
				//println("Error Write:", err.Error())
				httpConnectionSetState(httpConnectionInfo, HttpRequestStateWriteError)
				httpConnectionInfo.ioError = err
			}
		}

		write_total_bytes = write_total_bytes + write_round_bytes
		//fmt.Printf("writing %v(%v), chunk write %v\r\n", write_total_bytes, text_len, write_round_bytes)
		if write_total_bytes == text_len {
			//fmt.Printf("DONE write %v(%v), chunk write %v\r\n", write_total_bytes, text_len, write_round_bytes)
			httpConnectionSetState(httpConnectionInfo, HttpRequestStateWriteDone)
			return
		}
	}
	//io error
	return
}

//http reaponse server Goroutine
func httpSendResponse(httpConnectionInfo *httpConnectionInfo_t) {
	/*
		read http request and send back dyn text:
		GET / HTTP/1.0
		Host: localhost

		----

		HTTP/1.1 200 OK
		Cache-Control: no-cache, must-revalidate
		Content-Type: text/plain; charset=UTF-8
		Expires: Fri, 01 Jan 1990 00: 00: 00 GMT
		Date: Fri, 11 Jan 2013 09:18:46 GMT
		Pragma: no-cache
		Server: Wheelcomplex-Go-Qps-Server v1.0
		Content-Length: $LEN

		Connection:keep-alive
		Connection: close

		Wheelcomplex-Go-Qps-Server v1.0, runtime mode <readlonly/short/keepalive>
	*/

	//responsetHeaderList no use yet

	httpConnectionInfo.responseBodyText = append(httpConnectionInfo.responseBodyText, []byte("/"+time.Now().UTC().Format(HttpTimeFormat)+"/"+strconv.Itoa(httpConnectionInfo.requestCount)+"\r\n")...)

	switch srv_cfg_mode {
	case "readlonly":
		//no response for readonly mode
		return
	case "short":
		//LEARN
		httpConnectionInfo.keepalive = false
		//slice = append(slice, anotherSlice...), DO NOT FORGOT the last '...'
		httpConnectionInfo.responseHeaderText = append(httpConnectionInfo.responseHeaderText, []byte("HTTP/1.0"+httpStatusText[httpConnectionInfo.responseStatusCode]+"\r\n")...)
		httpConnectionInfo.responseHeaderText = append(httpConnectionInfo.responseHeaderText, []byte("Connection: close\r\n")...)
	default:
		//case "keepalive":
		httpConnectionInfo.responseHeaderText = append(httpConnectionInfo.responseHeaderText, []byte("HTTP/1.1"+httpStatusText[httpConnectionInfo.responseStatusCode]+"\r\n")...)
		if httpConnectionInfo.responseStatusCode < httpStatusBadRequest {
			httpConnectionInfo.keepalive = true
			httpConnectionInfo.responseHeaderText = append(httpConnectionInfo.responseHeaderText, []byte("Connection: keep-alive\r\n")...)
		} else {
			httpConnectionInfo.keepalive = false
			httpConnectionInfo.responseHeaderText = append(httpConnectionInfo.responseHeaderText, []byte("Connection: close\r\n")...)
		}
	}

	//static header
	//use a static line will save cpu but no clear
	httpConnectionInfo.responseHeaderText = append(httpConnectionInfo.responseHeaderText, []byte("Cache-Control: no-cache, must-revalidate\r\n")...)
	httpConnectionInfo.responseHeaderText = append(httpConnectionInfo.responseHeaderText, []byte("Content-Type: text/plain; charset=UTF-8\r\n")...)
	httpConnectionInfo.responseHeaderText = append(httpConnectionInfo.responseHeaderText, []byte("Pragma: no-cache\r\n")...)

	//Content-Length: $LEN
	httpConnectionInfo.responseHeaderText = append(httpConnectionInfo.responseHeaderText, []byte("Content-Length: "+strconv.Itoa(len(httpConnectionInfo.responseBodyText))+"\r\n")...)

	//Date: Fri, 11 Jan 2013 09:18:46 GMT
	httpConnectionInfo.responseHeaderText = append(httpConnectionInfo.responseHeaderText, []byte("Date: "+time.Now().UTC().Format(HttpTimeFormat)+"\r\n")...)
	httpConnectionInfo.responseHeaderText = append(httpConnectionInfo.responseHeaderText, []byte("Expires: Fri, 01 Jan 1990 00: 00: 00 GMT\r\n")...)
	httpConnectionInfo.responseHeaderText = append(httpConnectionInfo.responseHeaderText, []byte("Server: Wheelcomplex-Go-Qps-Server v1.0\r\n")...)

	//end of header
	httpConnectionInfo.responseHeaderText = append(httpConnectionInfo.responseHeaderText, []byte("\r\n")...)

	//fmt.Printf("Writing header, len %d, [%v]\r\n", len(httpConnectionInfo.responseHeaderText), string(httpConnectionInfo.responseHeaderText))
	//write header, blocking io
	httpWriteResponser(httpConnectionInfo, httpConnectionInfo.responseHeaderText, srv_cfg_net_timeout)
	switch httpConnectionInfo.state {
	case HttpRequestStateWriteDone:
		httpConnectionSetState(httpConnectionInfo, HttpRequestStateWriteHeaderDone)
	case HttpRequestStateWriteTimeOut:
		httpConnectionSetState(httpConnectionInfo, HttpRequestStateWriteHeaderTimeOut)
		return
	default:
		//error
		httpConnectionSetState(httpConnectionInfo, HttpRequestStateWriteHeaderError)
		return
	}

	//write delay
	if srv_cfg_write_delay > 0 {
		//fmt.Printf("Delay %v second befor writing body.\r\n", srv_cfg_write_delay)
		httpConnectionSetState(httpConnectionInfo, HttpRequestStateWriteWaiting)
		blockSeconds(srv_cfg_write_delay)
		//<-time.Tick(time.Duration(srv_cfg_write_delay) * time.Second)
		//time.Sleep(time.Duration(srv_cfg_write_delay) * time.Second)
		//fmt.Printf("Delay %v second done, writing body.\r\n", srv_cfg_write_delay)
	}
	//
	httpConnectionSetState(httpConnectionInfo, HttpRequestStateWriteBody)

	//fmt.Printf("Writing body, len %d, [%v]\r\n", len(httpConnectionInfo.responseBodyText), string(httpConnectionInfo.responseBodyText))
	httpWriteResponser(httpConnectionInfo, httpConnectionInfo.responseBodyText, srv_cfg_net_timeout)
	switch httpConnectionInfo.state {
	case HttpRequestStateWriteDone:
		httpConnectionSetState(httpConnectionInfo, HttpRequestStateWriteBodyDone)
		httpConnectionInfo.eventNotifyChannel <- TOTAL_CONN_RESPONSE
	case HttpRequestStateWriteTimeOut:
		httpConnectionSetState(httpConnectionInfo, HttpRequestStateWriteBodyTimeOut)
		return
	default:
		//error
		httpConnectionSetState(httpConnectionInfo, HttpRequestStateWriteBodyError)
		return
	}
	return
}

///////////////////////////////////////////////////////////////////////////////////////
//main
func main() {
	//
	commandArgsParser()
	//
	fmt.Fprintf(os.Stderr, optargex.VersionString())
	listener, err := net.Listen("tcp", srv_cfg_listen)
	if err != nil {
		println("error listening:", err.Error())
		os.Exit(1)
	}

	defer listener.Close()

	fmt.Fprintf(os.Stderr, "Max-connections %d, Using %d CPUS, mode %v, Listening at %s ...\r\n", srv_cfg_max_connections, runtime.GOMAXPROCS(-1), srv_cfg_mode, srv_cfg_listen)
	if srv_cfg_debug_level >= 3 {
		fmt.Fprintf(os.Stderr, "WARNNING: debug level %v.\r\n", srv_cfg_debug_level)
	}
	if srv_cfg_qps_interval < 1 {
		fmt.Fprintf(os.Stderr, "WARNNING: QPS interval display disabled.\r\n")
	}

	//+10240 slot to send 503 server busy
	var mainEventNotifyChannel chan int = make(chan int, srv_cfg_max_connections+10240)
	//var mainNewConnChannel chan int = make(chan int, srv_cfg_max_connections+10240)
	//var mainClosedConnChannel chan int = make(chan int, srv_cfg_max_connections+10240)
	var mainServerQuitChannel chan bool = make(chan bool)

	looping := true

	go func() {
		//timer tick
		for _ = range time.Tick(time.Second) {
			mainEventNotifyChannel <- TOTAL_TIME_ESP
		}
	}()

	var status_counter = make(map[int]int)
	for key, _ := range EventChangeString {
		status_counter[key] = 0
	}

	status_counter[TOTAL_CONN_MAX] = (srv_cfg_max_connections)
	status_counter[TOTAL_CONN_IDLE] = (srv_cfg_max_connections)
	go func() {
		//connections counter

		for eventChangeMsg := range mainEventNotifyChannel {
			if eventChangeMsg == HttpRequestStateDestroyed {
				//connection closed
				//status_counter[TOTAL_CONN_RUNNING]--
				atomic.AddInt32(&srv_cfg_var_conn_counter, -1)

				mainEventNotifyChannel <- TOTAL_CONN_CLOSED

				//fmt.Fprintf(os.Stderr, "new eventChangeMsg: #%v, %d/%s\r\n", eventChangeMsg, int(status_counter[eventChangeMsg]), EventChangeString[eventChangeMsg])
			}
			_, varItemExist := EventChangeString[eventChangeMsg]
			if varItemExist == false {
				EventChangeString[eventChangeMsg] = "unknow event no " + strconv.Itoa(eventChangeMsg)
			}

			status_counter[eventChangeMsg]++

		}
	}()

	if srv_cfg_qps_interval > 0 {
		//QPS counter, show interval: 5 seconds
		go func() {
			//
			var status_interval_qps float32
			var status_total_qps float32
			var status_snashot = make(map[int]int)
			var qps_time_string string
			var qps_string_buffer string
			var countDiff int
			var initPre int

			for _ = range time.Tick(time.Duration(srv_cfg_qps_interval) * time.Second) {

				qps_string_buffer = ""

				status_counter[TOTAL_CONN_RUNNING] = int(atomic.LoadInt32(&srv_cfg_var_conn_counter))
				status_counter[TOTAL_CONN_IDLE] = srv_cfg_max_connections - status_counter[TOTAL_CONN_RUNNING]

				qps_time_string = time.Now().Format(time.RFC1123)
				//here
				if status_counter[TOTAL_CONN_RUNNING] < srv_cfg_max_connections {
					qps_string_buffer = qps_string_buffer + fmt.Sprintf("STATUS[%s]: Connections avaible.\r\n", qps_time_string)
				} else {
					qps_string_buffer = qps_string_buffer + fmt.Sprintf("STATUS[%s]: Connections full.\r\n", qps_time_string)

				}

				//

				for key := EVENT_START; key < EVENT_LAST; key++ {
					if status_counter[key] == 0 {
						status_interval_qps = 0
						status_total_qps = 0
					} else {
						_, varItemExist := status_snashot[key]
						if varItemExist == false {
							status_snashot[key] = 0
						}

						countDiff = status_counter[key] - status_snashot[key]
						status_interval_qps = float32(countDiff) / float32(srv_cfg_qps_interval)
						//
						status_total_qps = float32(status_counter[key]) / float32(status_counter[TOTAL_TIME_ESP])
					}
					//
					switch {
					case key == EVENT_START:
					case key == EVENT_LAST:
					case key == TOTAL_TIME_ESP:
						fallthrough
					case key == TOTAL_CONN_MAX:
						fallthrough
					case key == TOTAL_CONN_RUNNING:
						fallthrough
					case key == TOTAL_CONN_IDLE:

						qps_string_buffer = qps_string_buffer + fmt.Sprintf("      [%s]: %d / %s\r\n", qps_time_string, int(status_counter[key]), EventChangeString[key])
					case key == TOTAL_CONN_INIT:
						fallthrough
					case key == TOTAL_CONN_CLOSED:
						fallthrough
					case key == TOTAL_CONN_REQUEST:
						fallthrough
					case key == TOTAL_CONN_KEEPALIVE:
						fallthrough
					case key == TOTAL_CONN_RESPONSE:
						qps_string_buffer = qps_string_buffer + fmt.Sprintf("      [%s]: %d / qps %f(total)/%f(%d seconds), %s\r\n", qps_time_string, int(status_counter[key]), status_total_qps, status_interval_qps, srv_cfg_qps_interval, EventChangeString[key])
					case status_counter[key] <= 0:
					default:
						if srv_cfg_debug_level > 1 {
							qps_string_buffer = qps_string_buffer + fmt.Sprintf("      [%s]: %d / %d = %f, %d / %d = %f / %s,\r\n", qps_time_string, int(status_counter[key]), int(status_counter[TOTAL_TIME_ESP]), status_total_qps, int(countDiff), srv_cfg_qps_interval, status_interval_qps, EventChangeString[key])
						}
					}
					status_snashot[key] = status_counter[key]
				}
				qps_string_buffer = qps_string_buffer + fmt.Sprintf("      [%s]: --------------------------\r\n", qps_time_string)
				if status_counter[TOTAL_CONN_REQUEST] != initPre {
					initPre = status_counter[TOTAL_CONN_REQUEST]
					fmt.Printf("%s", qps_string_buffer)
				}
			}
		}()
	}

	//blocking accept new connection
	go func() {
		//bodyText := []byte("Wheelcomplex-Go-Qps-Server v1.0, runtime mode " + srv_cfg_mode + ".\r\n")
		bodyText := []byte("mode " + srv_cfg_mode)
		var conn net.Conn
		var err error
		for {
			//accept new connetion, blocking IO
			conn, err = listener.Accept()
			if err != nil {
				//Accept error
				fmt.Printf("%s\r\n", err.Error())
				//shutdown server
				mainServerQuitChannel <- true
				return
			}
			//got new connection
			mainEventNotifyChannel <- TOTAL_CONN_INIT
			//status_counter[TOTAL_CONN_RUNNING]++
			atomic.AddInt32(&srv_cfg_var_conn_counter, 1)
			if int(srv_cfg_var_conn_counter) > srv_cfg_max_connections {
				//reach srv_cfg_max_connections, send busy message to client end close.
				//waiting for exist connection close

				//slow down Accept for anti-spam
				//time.Sleep(time.Second)
				//blockSeconds(1)
				go httpConnectionHandler(httpStatusServiceUnavailable, bodyText, conn, mainEventNotifyChannel)
			} else {
				//new request
				go httpConnectionHandler(httpStatusContinue, bodyText, conn, mainEventNotifyChannel)
			}
		}
	}()

	for looping {
		//
		//check quit flag
		select {
		case <-mainServerQuitChannel:
			looping = false
		default:
		}
		//do something else
		//time.Sleep(5 * time.Millisecond)
		blockSeconds(5)
		//<-time.Tick(time.Duration(5) * time.Second)
		runtime.Gosched()
	}
	//clean up server befor quit
	fmt.Fprintf(os.Stderr, "clean up server befor quit ...\n")
	//
	//time.Sleep(5 * time.Second)
	blockSeconds(5)
	//<-time.Tick(time.Duration(5) * time.Second)
	//
	return
}

//end
