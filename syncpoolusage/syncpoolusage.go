// sync.Pool usage test
//
// https://golang.org/pkg/sync/#Pool
//
// http://play.golang.org/p/KfAugjcH0e
//

package main

import (
	"runtime"
	"sync"
)

var testloop int = 10000000

// Thread Local Storage (TLS)
func pooltest(tag string, cpus int, lock bool, sched bool, loop int) {

	//
	runtime.GOMAXPROCS(cpus)

	runtime.UnlockOSThread()

	if lock {
		// make sure test running in the same os thread
		runtime.LockOSThread()
	}

	var count int

	var pool = sync.Pool{
		New: func() interface{} {
			count++
			buffer := make([]byte, 4096)
			return buffer
		},
	}
	count = 0
	for l := 0; l < loop; l++ {
		rawdata := pool.Get()
		data := rawdata.([]byte)
		// do something to data
		for idx, _ := range data {
			data[idx] = 0
		}
		pool.Put(rawdata)
		if sched {
			runtime.Gosched()
		}
	}
	println("Gosched", sched, "OSThread lock", lock, tag, "with ", runtime.GOMAXPROCS(-1), "CPUs, test1 count:", count)

	// use new pool
	pool = sync.Pool{
		New: func() interface{} {
			count++
			buffer := make([]byte, 4096)
			return buffer
		},
	}
	count = 0
	for l := 0; l < loop; l++ {
		data := pool.Get().([]byte)
		// do something to data
		for idx, _ := range data {
			data[idx] = 0
		}
		pool.Put(data)
		if sched {
			runtime.Gosched()
		}
	}
	println("Gosched", sched, "OSThread lock", lock, tag, "with ", runtime.GOMAXPROCS(-1), "CPUs, test2 count:", count)
	println("---")
	runtime.UnlockOSThread()
}

// make sure it's running in single thread
func init() {

	pooltest("run in init", 1, true, false, testloop)
	pooltest("run in init", 1, false, false, testloop)

	pooltest("run in init", 8, true, false, testloop)
	pooltest("run in init", 8, false, false, testloop)

	pooltest("run in init", 1, true, true, testloop)
	pooltest("run in init", 1, false, true, testloop)

	pooltest("run in init", 8, true, true, testloop)
	pooltest("run in init", 8, false, true, testloop)

}

func main() {
	pooltest("run in main", 1, true, false, testloop)
	pooltest("run in main", 1, false, false, testloop)
	pooltest("run in main", 8, true, false, testloop)
	pooltest("run in main", 8, false, false, testloop)

	pooltest("run in main", 1, true, true, testloop)
	pooltest("run in main", 1, false, true, testloop)
	pooltest("run in main", 8, true, true, testloop)
	pooltest("run in main", 8, false, true, testloop)
	// output:
	/*

	 */
}

//
