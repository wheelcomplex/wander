// concurrence read & write int64

package main

import (
	"math/rand"
	"runtime"
	"sync"
)

func randwrite(seed *int64, wg *sync.WaitGroup) {
	wg.Done()
	for {
		*seed = rand.Int63()%100 + 10
	}
}

func reader(seed *int64) {
	for {
		rd := *seed
		if rd > 0 && (rd < 10 || rd > 110) {
			println(rd)
		}
	}
}

func main() {

	runtime.GOMAXPROCS(8)

	println("testing with", runtime.GOMAXPROCS(-1), "concurrence routine .....")
	var seed int64 = 0
	wg := sync.WaitGroup{}
	for i := 0; i < runtime.GOMAXPROCS(-1); i++ {
		wg.Add(1)
		go randwrite(&seed, &wg)
	}
	// reader
	wg.Wait()
	for i := 0; i < runtime.GOMAXPROCS(-1); i++ {
		go reader(&seed)
	}
	reader(&seed)
}
