package main

import (
	"flag"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"sync"
	"time"
)

// fakeAES fill a tmp buffer and return
func fakeAES(data []byte, size int) []byte {
	tmp := make([]byte, size)
	ptr := 0
	blocklen := len(tmp)
	for idx, _ := range data {
		data[idx] = tmp[ptr]
		ptr++
		if ptr == blocklen {
			ptr = 0
		}
	}
	return data
}

// poolAES fill a pool buffer and return
func poolAES(data []byte, tmp []byte) []byte {
	ptr := 0
	blocklen := len(tmp)
	for idx, _ := range data {
		data[idx] = tmp[ptr]
		ptr++
		if ptr == blocklen {
			ptr = 0
		}
	}
	return data
}

func main() {
	var profileport = flag.Int("port", 6060, "profile http port")
	var shake = flag.Int("buffer", 2*256, "shake buffer size")
	var loop = flag.Int("loop", 3e7, "shake loops")
	var syncpool = flag.Bool("pool", false, "enable sync pool")
	var dummy = flag.Bool("dummy", false, "enable dummy buffer")
	flag.Parse()
	fmt.Printf("go tool pprof http://localhost:%d/debug/pprof/profile\n", *profileport)
	go func() {
		fmt.Println(http.ListenAndServe(fmt.Sprintf("localhost:%d", *profileport), nil))
	}()

	uuu := uint8(0)

	fmt.Printf("UUU %d\n", uuu-1)

	// dummy memory, 80mb
	dummysize := 800 * 1024 * 1024
	if *dummy {
		dbuf := make([]byte, dummysize)
		for idx, _ := range dbuf {
			dbuf[idx] = 0
		}
	}

	data := make([]byte, 3*256)
	start := time.Now()
	if *syncpool {
		// fill pool
		pool := sync.Pool{}
		pb := make([]byte, *shake)
		pool.Put(pb)
		for i := 0; i < *loop; i++ {
			pb = pool.Get().([]byte)
			poolAES(data, pb)
			pool.Put(pb)
		}
	} else {
		for i := 0; i < *loop; i++ {
			fakeAES(data, *shake)
		}
	}
	esp := time.Now().Sub(start)
	qps := float32(*loop) / float32(esp.Seconds())
	fmt.Printf("sync pool %v, dummy %v, count %d, esp %v, qps %f\n", *syncpool, *dummy, *loop, esp, qps)
}
