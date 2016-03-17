// testing runtime.SetPanicFlag,try to use all free memory for panic: out of memory

package main

import "runtime"

func main() {
	runtime.SetPanicFlag(2)
	for k := 1; k < 1e9; k++ {
		buf := make([]byte, k)
		go func(buf []byte) {
			for i := 0; i < len(buf); i++ {
				buf[i] = 0
			}
			// hold memory
			select {}
		}(buf)
		//print(k, "\n")
	}
}
