package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	//	"runtime"
	"bytes"
	"time"
)

var path string = "./"

func main() {
	startTime := time.Now().UnixNano()
	//解析
	filenameList := []string{"test_log2.log"}
	//	runtime.GOMAXPROCS(runtime.NumCPU())
	for _, file := range filenameList {
		parse(file)
	}
	endTime := time.Now().UnixNano()
	fmt.Println(startTime)
	fmt.Println(endTime)
	fmt.Println("运行时间(optimated):", (endTime-startTime)/1000/1000, "毫秒")
}

func parse(file string) {
	filePath := path + file
	f, err := os.Open(filePath)
	defer f.Close()
	if err != nil {
		fmt.Println("打开文件失败:" + filePath)
		os.Exit(1)
	}
	//错误信息
	var errList []error
	//总流量
	var totalSize uint64

	var errMsg error
	var line []byte
	var lineNum uint64 = 0

	var keepStopBuf = []byte("keep_stopp=on")

	bfRd := bufio.NewReader(f)
	for {
		lineNum += 1
		line, errMsg = bfRd.ReadSlice('\n')
		//判断是否结尾
		if errMsg == io.EOF {
			fmt.Println("有效大小:", totalSize, "B")
			return
		}
		if bytes.Contains(line, keepStopBuf) {
			continue
		}
		slice := bytes.SplitN(line, []byte(" "), 10)
		if len(slice) < 9 {
			continue
		}
		size, _ := strconv.ParseInt(string(slice[8]), 10, 0)
		// =

		totalSize += uint64(size)
		if errMsg != nil {
			errList = append(errList, errMsg)
			continue
		}
	}
}
