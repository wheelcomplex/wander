/*
	TCP Connection test
*/

package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"time"
)

func TimePrintf(format string, v ...interface{}) {
	ts := fmt.Sprintf("[%s] ", time.Now().String())
	msg := fmt.Sprintf(format, v...)
	fmt.Printf("%s%s", ts, msg)
}

func main() {
	mode := flag.String("m", "server", "run mode")
	rbufsize := flag.Int("R", 1, "read buffer size")
	rtime := flag.Int("T", 65535, "read ttimeout")
	flag.Parse()
	hostport := "127.0.0.1:8800"
	if *mode == "server" {
		laddr, lerr := net.ResolveTCPAddr("tcp", hostport)
		if lerr != nil {
			println(lerr.Error())
			os.Exit(9)
		}
		l, lerr := net.ListenTCP("tcp", laddr)
		if lerr != nil {
			println(lerr.Error())
			os.Exit(9)
		}
		conn, _ := l.AcceptTCP() // *net.TCPConn
		fmt.Printf("[%d]new conn %s\n", os.Getpid(), conn.RemoteAddr().String())
		conn.SetWriteBuffer(5)
		buf := []byte("01234567890123456789")
		for i := 0; i < 10; i++ {
			_, err := conn.Write(buf[i : i+1])
			if err != nil {
				fmt.Printf("out#%d: %s\n", i, err.Error())
				break
			} else {
				fmt.Printf("out#%d: %s\n", i, buf[i:i+1])
			}
		}
		conn.Close()
		println("server closed")
		l.Close()
		return
	}
	client, ce := net.Dial("tcp4", hostport)
	if ce != nil {
		TimePrintf("ERROR: %s\n", ce.Error())
		os.Exit(1)
	}
	TimePrintf("conected: %s\n", hostport)

	println("reading with buffer size:", *rbufsize, "timeout", *rtime)
	buflen := *rbufsize
	rbuf := make([]byte, buflen)
	tbuf := make([]byte, 0, 100*buflen)
	// read until read timeout
	// wait one seconds for server closed
	time.Sleep(5e9)
	cl := client.(*net.TCPConn)
	cl.SetReadBuffer(1)
	for {
		nr, re := cl.Read(rbuf)
		if nr > 0 {
			tbuf = append(tbuf, rbuf[:nr]...)
		}
		if re != nil {
			TimePrintf("ERROR: read %s failed: %s\n", hostport, re.Error())
			break
		}
		time.Sleep(1e8)
	}
	if len(tbuf) > 0 {
		TimePrintf("[%d]Got final response %s (%d)\n ------ \n%s\n", os.Getpid(), hostport, len(tbuf), tbuf)
		TimePrintf(" ------\n")
	}
	cl.Close()
	return
}

//
