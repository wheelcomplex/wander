// stackSplitTest
package main

import (
	"fmt"
	"time"
)

//测试函数嵌套多少次后，进行runtion.stackSplit，导致运行效率急剧下降

var C = make(chan int, 100)
var F = "嵌套次数%5d,From=%5d,To=%5d,Group=%6d,耗时%7s毫秒\t%d\n"

//计算函数
func sum(from int, to int) (r int) {
	r = 0
	for i := from; i <= to; i++ {
		r += i
	}
	return
}

//嵌套调用
func NestedCall1(n int, from int, to int, grp int,
	a01, a02, a03, a04, a05, a06, a07, a08, a09, a10,
	a11, a12, a13, a14, a15, a16, a17, a18, a19, a20,
	a21, a22, a23, a24, a25, a26, a27, a28, a29, a30,
	a31, a32, a33, a34, a35, a36, a37, a38, a39, a40,
	a41, a42, a43, a44, a45, a46, a47, a48, a49, a50,
	a51, a52, a53, a54, a55, a56, a57, a58, a59, a60,
	a61, a62, a63, a64, a65, a66, a67, a68, a69, a70,
	a71, a72, a73, a74, a75, a76, a77, a78, a79, a80 int) (r int) {
	if n <= 0 {
		r = 0
		for g := 0; g < grp; g++ {
			r += sum(from, to)
		}
		return
	} else {
		return NestedCall1(n-1, from, to, grp,
			a01, a02, a03, a04, a05, a06, a07, a08, a09, a10,
			a11, a12, a13, a14, a15, a16, a17, a18, a19, a20,
			a21, a22, a23, a24, a25, a26, a27, a28, a29, a30,
			a31, a32, a33, a34, a35, a36, a37, a38, a39, a40,
			a41, a42, a43, a44, a45, a46, a47, a48, a49, a50,
			a51, a52, a53, a54, a55, a56, a57, a58, a59, a60,
			a61, a62, a63, a64, a65, a66, a67, a68, a69, a70,
			a71, a72, a73, a74, a75, a76, a77, a78, a79, a80)
	}
}

func NestedTest1() {
	for i := 0; i <= 500; i++ {
		begin := time.Now()
		N := NestedCall1(i, 1, 10, 1000000,
			1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
			11, 12, 13, 14, 15, 16, 17, 18, 19, 20,
			21, 22, 23, 24, 25, 26, 27, 28, 29, 30,
			31, 32, 33, 34, 35, 36, 37, 38, 39, 40,
			41, 42, 43, 44, 45, 46, 47, 48, 49, 50,
			51, 52, 53, 54, 55, 56, 57, 58, 59, 60,
			61, 62, 63, 64, 65, 66, 67, 68, 69, 70,
			71, 72, 73, 74, 75, 76, 77, 78, 79, 80)
		end := time.Now()
		use := int64(end.Sub(begin).Seconds() * 1000)
		if use > 9 || i < 5 {
			fmt.Printf(F, i, 1, 1000, 100, fmt.Sprint(use), N)
		}
	}
	C <- 0
}

//嵌套调用
func NestedCall2(n int, from int, to int, grp int,
	a01, a02, a03, a04, a05, a06, a07, a08, a09, a10,
	a11, a12, a13, a14, a15, a16, a17, a18, a19, a20,
	a21, a22, a23, a24, a25, a26, a27, a28, a29, a30,
	a31, a32, a33, a34, a35, a36, a37, a38, a39, a40,
	a41, a42, a43, a44, a45, a46, a47, a48, a49, a50,
	a51, a52, a53, a54, a55, a56, a57, a58, a59, a60,
	a61, a62, a63, a64, a65, a66, a67, a68, a69, a70,
	a71, a72, a73, a74, a75, a76, a77, a78, a79, a80 int, s string) (r int) {
	if n <= 0 {
		r = 0
		for g := 0; g < grp; g++ {
			r += sum(from, to)
		}
		return
	} else {
		return NestedCall2(n-1, from, to, grp,
			a01, a02, a03, a04, a05, a06, a07, a08, a09, a10,
			a11, a12, a13, a14, a15, a16, a17, a18, a19, a20,
			a21, a22, a23, a24, a25, a26, a27, a28, a29, a30,
			a31, a32, a33, a34, a35, a36, a37, a38, a39, a40,
			a41, a42, a43, a44, a45, a46, a47, a48, a49, a50,
			a51, a52, a53, a54, a55, a56, a57, a58, a59, a60,
			a61, a62, a63, a64, a65, a66, a67, a68, a69, a70,
			a71, a72, a73, a74, a75, a76, a77, a78, a79, a80, s)
	}
}
func NestedTest2() {
	for i := 0; i <= 500; i++ {
		begin := time.Now()
		N := NestedCall2(i, 1, 10, 1000000,
			1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
			11, 12, 13, 14, 15, 16, 17, 18, 19, 20,
			21, 22, 23, 24, 25, 26, 27, 28, 29, 30,
			31, 32, 33, 34, 35, 36, 37, 38, 39, 40,
			41, 42, 43, 44, 45, 46, 47, 48, 49, 50,
			51, 52, 53, 54, 55, 56, 57, 58, 59, 60,
			61, 62, 63, 64, 65, 66, 67, 68, 69, 70,
			71, 72, 73, 74, 75, 76, 77, 78, 79, 80,
			"                                                                 ")
		end := time.Now()
		use := int64(end.Sub(begin).Seconds() * 1000)
		if use > 9 || i < 5 {
			fmt.Printf(F, i, 1, 1000, 100, fmt.Sprint(use), N)
		}
	}
	C <- 0
}

func NestedCall3(n int, from int, to int, grp int,
	a01, a02, a03, a04, a05, a06, a07, a08, a09, a10,
	a11, a12, a13, a14, a15, a16, a17, a18, a19, a20,
	a21, a22, a23, a24, a25, a26, a27, a28, a29, a30,
	a31, a32, a33, a34, a35, a36, a37, a38, a39, a40,
	a41, a42, a43, a44, a45, a46, a47, a48, a49, a50,
	a51, a52, a53, a54, a55, a56, a57, a58, a59, a60,
	a61, a62, a63, a64, a65, a66, a67, a68, a69, a70,
	a71, a72, a73, a74, a75, a76, a77, a78, a79, a80 int, s interface{}) (r int) {
	if n <= 0 {
		r = 0
		for g := 0; g < grp; g++ {
			r += sum(from, to)
		}
		return
	} else {
		return NestedCall3(n-1, from, to, grp,
			a01, a02, a03, a04, a05, a06, a07, a08, a09, a10,
			a11, a12, a13, a14, a15, a16, a17, a18, a19, a20,
			a21, a22, a23, a24, a25, a26, a27, a28, a29, a30,
			a31, a32, a33, a34, a35, a36, a37, a38, a39, a40,
			a41, a42, a43, a44, a45, a46, a47, a48, a49, a50,
			a51, a52, a53, a54, a55, a56, a57, a58, a59, a60,
			a61, a62, a63, a64, a65, a66, a67, a68, a69, a70,
			a71, a72, a73, a74, a75, a76, a77, a78, a79, a80, s)
	}
}
func NestedTest3() {
	for i := 0; i <= 500; i++ {
		begin := time.Now()
		N := NestedCall3(i, 1, 10, 1000000,
			1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
			11, 12, 13, 14, 15, 16, 17, 18, 19, 20,
			21, 22, 23, 24, 25, 26, 27, 28, 29, 30,
			31, 32, 33, 34, 35, 36, 37, 38, 39, 40,
			41, 42, 43, 44, 45, 46, 47, 48, 49, 50,
			51, 52, 53, 54, 55, 56, 57, 58, 59, 60,
			61, 62, 63, 64, 65, 66, 67, 68, 69, 70,
			71, 72, 73, 74, 75, 76, 77, 78, 79, 80,
			"                                                                 ")
		end := time.Now()
		use := int64(end.Sub(begin).Seconds() * 1000)
		if use > 9 || i < 5 {
			fmt.Printf(F, i, 1, 1000, 100, fmt.Sprint(use), N)
		}
	}
	C <- 0
}

func main() {
	fmt.Println("----------NestedTest1----------")
	go NestedTest1()
	<-C
	fmt.Println("----------NestedTest2----------")
	go NestedTest2()
	<-C
	fmt.Println("----------NestedTest3----------")
	go NestedTest3()
	<-C
}
