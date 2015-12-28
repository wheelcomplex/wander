// testing huge wait on channel and close signal

// BUG: -c 1000000 it's ok, signal and exit worked, -c 8000000 count down to 1,2,3,4 and freeze

package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

var showhelp bool

var max int64

var wait int

var sigChan = make(chan struct{}, 0)

var count *int64

func main() {

	flag.BoolVar(&showhelp, "h", false, "show help")

	flag.Int64Var(&max, "c", 8000000, "Concurrent waiting goroutine")

	flag.IntVar(&wait, "w", 5, "waiting for N seconds and close channel")

	flag.Parse()

	if showhelp || max <= 0 {
		flag.PrintDefaults()
		os.Exit(1)
	}

	httpProfile("127.0.0.1:6066")

	var wg sync.WaitGroup
	// WaitGroup is too slow for counting, atomic used
	var int64Count int64
	count = &int64Count

	fmt.Printf("creating %d goroutine...\n", max)
	wg.Add(1)
	for idx := int64(0); idx < max; idx++ {
		go runner(&wg)
	}
	atomic.AddInt64(count, max)
	fmt.Printf("%d goroutine waiting for %d seconds ...\n", max, wait)
	go func() {
		time.Sleep(time.Duration(wait) * time.Second)
		close(sigChan)
		fmt.Printf("closed, bye-bye.\n")
	}()
	go func() {
		tk := time.NewTicker(time.Second)
		for i := 0; i < wait; i++ {
			<-tk.C
			fmt.Println(i)
		}
		tk.Stop()
	}()
	wg.Wait()
	fmt.Printf("all goroutine exited.\n")
}

func runner(wg *sync.WaitGroup) {
	// waiting for close
	<-sigChan
	if atomic.AddInt64(count, -1) == 0 {
		// all exit, send done notify
		wg.Done()
	}
}

func httpProfile(profileurl string) {
	binpath, _ := filepath.Abs(os.Args[0])
	fmt.Printf("\nprofile: [ http://%s/debug/pprof/ ]\n", profileurl)
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
//
//
