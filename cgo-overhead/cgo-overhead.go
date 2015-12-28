// CGO overhead test
package main

import (
	"flag"
	"fmt"
	"time"
)

/*
#include <stdio.h>

void add(int k) {
   k++;
}

*/
import "C"

//import "fmt"

func add(k int) {
	k++
}

var calls = flag.Int("n", 1e8, "number of func calls")

func main() {
	flag.Parse()
	fmt.Printf("CGO vs NATIVE, %d calls testing ...\n", *calls)
	now := time.Now()
	for i := 0; i < *calls; i++ {
		C.add(C.int(i))
	}
	c_dur_time := time.Now().Sub(now)
	fmt.Printf("CGO : %v\n", c_dur_time)
	now = time.Now()
	for i := 0; i < *calls; i++ {
		add(i)
	}
	g_dur_time := time.Now().Sub(now)
	fmt.Printf("NATIVE: %v\n", g_dur_time)
	fmt.Printf("DIFF: CGO %v - NATIVE %v = %v, CGO / NATIVE = %v \n",
		c_dur_time, g_dur_time, (c_dur_time - g_dur_time), int64(c_dur_time/g_dur_time))
}
