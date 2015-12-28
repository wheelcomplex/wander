package main

import (
	"fmt"
	"net"
	"os"
	"time"
)

func main() {
	laddr, lerr := net.ResolveTCPAddr("tcp", "127.0.0.1:8800")
	if lerr != nil {
		println(lerr.Error())
		os.Exit(9)
	}
	l, lerr := net.ListenTCP("tcp", laddr)
	if lerr != nil {
		println(lerr.Error())
		os.Exit(9)
	}
	synch := make(chan struct{}, 1)
	go func() {
		synch <- struct{}{}
		conn, _ := l.AcceptTCP() // *net.TCPConn
		defer conn.Close()
		fmt.Printf("new conn %s\n", conn.RemoteAddr().String())
		buf := []byte("0123456789")
		for i := 0; i < 10; i++ {
			<-synch
			_, err := conn.Write(buf[i : i+1])
			if err != nil {
				fmt.Printf("out#%d: %s\n", i, err.Error())
				break
			} else {
				fmt.Printf("out#%d: %s\n", i, buf[i:i+1])
			}
		}
	}()
	defer l.Close()
	// wait for listener
	<-synch
	nfd, err := net.Dial("tcp", "127.0.0.1:8800")
	if err != nil {
		println(err.Error())
		os.Exit(9)
	}
	defer nfd.Close()
	limit := time.Now().Add(3e9)
	nfd.SetDeadline(limit)
	rbuf := make([]byte, 10)
	tk := time.NewTicker(1e9)
	defer tk.Stop()
	synch <- struct{}{}
	for i := 0; i < 5; i++ {
		nr, ne := nfd.Read(rbuf)
		fmt.Printf("read %d byte, err %v: %v\n", nr, ne, rbuf[:nr])
		if ne != nil {
			break
		}
		synch <- struct{}{}
		<-tk.C
	}
	<-tk.C
}
