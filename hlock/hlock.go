package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"

	"titans/inbox"

	"github.com/hprose/hprose-go/hprose"
)

var storage *inbox.Storage

var data = flag.String("data", "./data", "path to data file")
var addr = flag.String("addr", "tcp://0.0.0.0:8087/", "addr to listen")

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	log.SetFlags(log.LstdFlags | log.Llongfile)
	log.SetOutput(os.Stderr)

	go func() {
		log.Println(http.ListenAndServe(":6060", nil))
	}()
	flag.Parse()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill)

	storage, _ = inbox.NewStorage(*data)

	r := inbox.NewRpc(storage)

	server := hprose.NewTcpServer(*addr)
	server.AddMethods(r)
	err := server.Start()
	if err != nil {
		fmt.Println(err)
		return
	}
	<-c
	server.Stop()
}
