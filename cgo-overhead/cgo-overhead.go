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
	now := time.Now()
	for i := 0; i < *calls; i++ {
		C.add(C.int(i))
	}
	c_dur_time := time.Now().Sub(now)
	fmt.Printf("CGO %d calls: CGO %v\n", *calls, c_dur_time.String())
	now = time.Now()
	for i := 0; i < *calls; i++ {
		add(i)
	}
	g_dur_time := time.Now().Sub(now)
	fmt.Printf("NATIVE %d calls: NATIVE %v\n", *calls, g_dur_time.String())
	fmt.Printf("CGO %d calls: CGO %v - NATIVE %v = %v, CGO / NATIVE = %v \n", *calls, c_dur_time, g_dur_time,
		(c_dur_time - g_dur_time).String(), int64(c_dur_time/g_dur_time))
}
