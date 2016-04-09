//
package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"sync/atomic"
)

func main() {
	fmt.Printf("runtime.GOMAXPROCS(-1) return %d CPUs\n", runtime.GOMAXPROCS(-1))
	fmt.Printf("runtime.NumCPU() return %d CPUs\n", runtime.NumCPU())
	cpuset := exec.Command("cpuset", "-g", "-p", strconv.Itoa(os.Getpid()))
	output, err := cpuset.CombinedOutput()
	if err != nil {
		fmt.Printf("exec cpuset failed: %s\n", err.Error())
		os.Exit(1)
	}
	fmt.Printf("cpuset return: %s\n", output)
	var cnt int64
	for i := 0; i < runtime.NumCPU(); i++ {
		go func() {
			for {
				atomic.AddInt64(&cnt, 1)
			}
		}()
	}
	fmt.Printf("%d CPU burnner running ...\n", runtime.NumCPU())
	fmt.Printf("Press <ctl>+C to exit ...\n")
	select {}
}
