/*
	TCP write test

	LAN link should be:  tester --- router --- WAN(ethernet)

	tester will write done without error even if WAN link unplugged

	(can not run in go play for networking limit)
*/

package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

func TimePrintf(format string, v ...interface{}) {
	ts := fmt.Sprintf("[%s] ", time.Now().String())
	msg := fmt.Sprintf(format, v...)
	fmt.Printf("%s%s", ts, msg)
}

func main() {
	doread := flag.Bool("r", true, "read response")
	//hostport := flag.String("H", "www.google.com:80", "connect to host")
	hostport := flag.String("H", "216.58.221.132:80", "connect to host")
	flag.Parse()
	if strings.Index(*hostport, ":") == -1 {
		*hostport += ":80"
	}
	TimePrintf("connecting to %s ...\n", *hostport)
	cl, ce := net.DialTimeout("tcp", *hostport, 5e9)
	if ce != nil {
		TimePrintf("ERROR: %s\n", ce.Error())
		os.Exit(1)
	}
	TimePrintf("conected: %s\n", *hostport)

	readit := make(chan struct{}, 1)
	readok := make(chan struct{}, 1)
	if *doread {
		go func() {
			var rtimeout time.Duration = 5e8
			buflen := 81920
			rbuf := make([]byte, buflen)
			for {
				<-readit
				// read until read timeout
				total := 0
				for {
					cl.SetReadDeadline(time.Now().Add(rtimeout))
					nr, re := cl.Read(rbuf[total:])
					if nr > 0 {
						total += nr
					}
					if re != nil {
						TimePrintf("ERROR: read %s failed: %s\n", *hostport, re.Error())
						if ne, ok := re.(net.Error); ok {
							if ne.Timeout() {
								break
							}
						}
						TimePrintf("Exit for read error\n")
						os.Exit(8)
					}
					if total >= buflen {
						TimePrintf("Got part response %s (%d)\n ------ \n%s\n", *hostport, total, rbuf[:total])
						TimePrintf(" ------\n")
						total = 0
					}
				}
				if total > 0 {
					TimePrintf("Got final response %s (%d)\n ------ \n%s\n", *hostport, total, rbuf[:total])
					TimePrintf(" ------\n")
				}
				readok <- struct{}{}
			}
		}()
	}
	wbuf := []byte("GET / HTTP/1.1\n")
	ebuf := []byte("Host: 127.0.0.1\n\n")
	loop := 0
	rbuf := make([]byte, 128)
	TimePrintf("unplugged WAN link of your router befor press <ENTER>, press <CTRL>-C to exit\n")
	_, re := os.Stdin.Read(rbuf)
	if re != nil {
		TimePrintf("ERROR: read stdin failed: %s\n", re.Error())
		os.Exit(1)
	}
	maxwrite := 3
	for {
		loop++
		if loop >= maxwrite {
			if loop == maxwrite {
				TimePrintf("read only for reach max write %d.\n", maxwrite)
			}
			readit <- struct{}{}
			<-readok
			time.Sleep(5e9)
			continue
		}
		nr := len(wbuf)
		TimePrintf("writing to %s(%d): %s\n", *hostport, nr, wbuf[:nr])
		// Write(b []byte) (n int, err error)
		nw, ew := cl.Write(wbuf[:nr])
		if ew != nil {
			TimePrintf("ERROR: %s\n", ew.Error())
			os.Exit(1)
		}
		TimePrintf("write done %s(%d/%d): %s\n", *hostport, nw, nr, wbuf[:nr])
		if *doread {
			readit <- struct{}{}
			<-readok
		}
		nr = len(ebuf)
		TimePrintf("writing to %s(%d): %s\n", *hostport, nr, ebuf[:nr])
		// Write(b []byte) (n int, err error)
		nw, ew = cl.Write(ebuf[:nr])
		if ew != nil {
			TimePrintf("ERROR: %s\n", ew.Error())
			os.Exit(1)
		}
		TimePrintf("write done %s(%d/%d): %s\n", *hostport, nw, nr, ebuf[:nr])
		if *doread {
			readit <- struct{}{}
			<-readok
		}
		time.Sleep(5e9)
	}
}
