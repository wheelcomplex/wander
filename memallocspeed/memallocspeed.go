// for https://groups.google.com/forum/#!topic/golang-nuts/HZ_sBoLtnX4
package main

import (
	"fmt"
	"time"
)

func domemalloc() {
	defer func() {
		if err := recover(); err != nil {
			println("panic for", err)
		}
	}()
	slimslice := make([]uint64, 256, 256)
	fatslice := make([]uint64, 0, 1024*1024*1024/8)
	start := time.Now()
	for {
		fatslice = append(fatslice, slimslice...)
		if len(fatslice)+len(slimslice) > cap(fatslice) {
			size := len(fatslice) * 8
			t := time.Since(start).Seconds()
			fmt.Printf("%d MB at %.2f = %.2f MB/s\n", size>>20, t, float64(size)/(1<<20)/t)
		}
	}
}
func main() {
	domemalloc()
}
