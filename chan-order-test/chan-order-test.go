package main

import (
	"flag"
	"math"
	"runtime"
	"sync"
)

func main() {
	cpus := flag.Int("c", 1, "using cpus")
	size := flag.Int("s", 0, "chan buffer size")
	flag.Parse()
	if *size <= 0 {
		*size = 0
	}
	a := make(chan uint64, *size)
	b := make(chan uint64, *size)
	c := make(chan struct{}, 128)
	if *cpus <= 0 {
		*cpus = runtime.NumCPU()
	}
	runtime.GOMAXPROCS(*cpus)
	println("chan buffer size", *size, "using", *cpus, "CPUs")
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := uint64(0); i < math.MaxUint64; i++ {
			a <- 1
			b <- 1
			c <- struct{}{}
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			oa := uint64(0)
			ob := uint64(0)
			for oa == 0 || ob == 0 {
				select {
				case oa = <-a:
				case ob = <-b:
				}
				if oa == 0 && ob == 1 {
					println("chan b first")
				}
			}
			<-c
		}
	}()
	wg.Wait()
}
