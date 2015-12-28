// sync.Pool test
//
// https://golang.org/pkg/sync/#Pool
//
// http://play.golang.org/p/hrRW2whH-_
//

package main

import (
	"runtime"
	"sync"
)

func main() {

	/*
		note1: loop less then 10000000 will case test2 Pool new counter always == 1
		note2: when set runtime.GOMAXPROCS(1), Pool new counter always == 1
		note3: http://play.golang.org/ is too slow to run this test(process took too long)
	*/

	var loop int = 10000000

	// running on multi-osthread
	runtime.GOMAXPROCS(16)

	var counter int

	var pool = sync.Pool{
		New: func() interface{} {
			counter++
			buffer := make([]byte, 4096)
			return buffer
		},
	}

	// reset counter
	counter = 0
	for l := 0; l < loop; l++ {
		rawdata := pool.Get()
		data := rawdata.([]byte)
		// do something to data
		for idx, _ := range data {
			data[idx] = 0
		}
		pool.Put(rawdata)
	}
	println(runtime.GOMAXPROCS(-1), "CPUs, test1 counter:", counter)

	// use new pool
	pool = sync.Pool{
		New: func() interface{} {
			counter++
			buffer := make([]byte, 4096)
			return buffer
		},
	}

	// reset counter
	counter = 0
	for l := 0; l < loop; l++ {
		data := pool.Get().([]byte)
		// do something to data
		for idx, _ := range data {
			data[idx] = 0
		}
		pool.Put(data)
	}
	println(runtime.GOMAXPROCS(-1), "CPUs, test2 counter:", counter)
	// output:
	/*
		16 CPUs, test1 counter: 1
		16 CPUs, test2 counter: 20
	*/
}

//
