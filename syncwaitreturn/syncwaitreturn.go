// sync wait return condition

package main

import (
	"runtime"
	"sync"
	"time"
)

func msdelay() {
	workMax := 58000000
	var i int
	var x float64
	for i = 0; i < workMax; i++ {
		x = 3.1415 / float64(i)
	}
	_ = x
}

func main() {

	runtime.GOMAXPROCS(8)

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		time.Sleep(100 * time.Millisecond)
		wg.Done()
		println("mama, I am done!")
		// starts := time.Now()
		msdelay()
		// fmt.Printf("%v\n", time.Now().Sub(starts))
		println("mama, I can fly!")
	}()
	wg.Wait()
	println("main exited.")
}
