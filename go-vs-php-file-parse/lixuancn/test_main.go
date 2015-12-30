package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
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
	fmt.Println("运行时间:", (endTime-startTime)/1000/1000, "毫秒")
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
	//过滤流量
	var keepStoppSize uint64

	var errMsg error
	var line []byte
	var size uint64
	var sizeTmp int64
	var lineNum uint64 = 0

	bfRd := bufio.NewReader(f)
	for {
		lineNum += 1
		line, errMsg = bfRd.ReadSlice('\n')
		//判断是否结尾
		if errMsg == io.EOF {
			fmt.Println("有效大小:", totalSize, "B")
			fmt.Println("已过滤大小:", keepStoppSize, "B")
			return
		}

		slice := bytes.SplitN(line, []byte(" "), 10)
		sizeTmp, _ = strconv.ParseInt(string(slice[8]), 10, 0)
		size = uint64(sizeTmp)
		if strings.Contains(string(slice[5]), "keep_stopp=on") {
			keepStoppSize += size
			continue
		}
		totalSize += size
		if errMsg != nil {
			errList = append(errList, errMsg)
			continue
		}
	}
}
