//

// C slice and Go slice convert
// https://github.com/golang/go/wiki/cgo
// http://blog.su21.org/post/play_with_cgo
package main

/*
#cgo CFLAGS: -L${SRCDIR}/ -std=gnu99
#include "cgo-array.h"
*/
import "C"
import (
	"fmt"
	"reflect"
	"unsafe"
)

func main() {
	fmt.Printf("show Go slice in C func\n")
	var length = 10
	var array []uint32 = make([]uint32, length)
	for i := 0; i < length; i++ {
		fmt.Printf("Go.array_show(%d): %d -> %d\n", length, i, array[i])
	}
	arrLen := C.uint32_t(length)
	arrPtr := (*C.uint32_t)(unsafe.Pointer(&array[0]))
	C.array_show(arrLen, arrPtr)

	fmt.Printf("modify Go slice in C func\n")
	C.array_modify(arrLen, arrPtr, 18)
	for i := 0; i < length; i++ {
		fmt.Printf("after modify, Go.array_show(%d): %d -> %d\n", length, i, array[i])
	}
	C.array_show(arrLen, arrPtr)

	// convert from C array/slice to Go slice
	fmt.Printf("create Go slice from C func\n")
	carrPtr := C.array_create(arrLen)
	defer C.free(unsafe.Pointer(carrPtr))
	fmt.Printf("created Go slice: %d || %p || %x || %v\n", carrPtr, carrPtr, carrPtr, carrPtr)
	hdr := reflect.SliceHeader{
		Data: uintptr(unsafe.Pointer(carrPtr)),
		Len:  length,
		Cap:  length,
	}
	goSlice := *(*[]C.uint32_t)(unsafe.Pointer(&hdr))
	// now goSlice is a Go slice backed by the C array
	for i := 0; i < length; i++ {
		fmt.Printf("after created, Go.array_show(%d): %d -> %d\n", length, i, goSlice[i])
	}
	arrPtr = (*C.uint32_t)(unsafe.Pointer(&goSlice[0]))
	fmt.Printf("modify Go slice(created by C) in C func\n")
	C.array_modify(arrLen, arrPtr, 18)
	for i := 0; i < length; i++ {
		fmt.Printf("after modify, Go.array_show(%d): %d -> %d\n", length, i, array[i])
	}
	C.array_show(arrLen, arrPtr)
}
