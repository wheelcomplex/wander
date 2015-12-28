/*
	channle+uint64 test
*/

//[2014-12-11 16:03:18.044853892 +0800 CST] Using 1/8 Cpus, using int64 false, buffer size 1024, time limit 10s
//[2014-12-11 16:03:18.045076897 +0800 CST]  ...
//[2014-12-11 16:03:28.045631991 +0800 CST] total, in 116680705(1166.741729/s), out 116680705(1166.741729/s), lost 0

package main

import (
	"fmt"
	"runtime"
	"time"

	"github.com/wheelcomplex/preinit"
)

func TimePrintf(format string, v ...interface{}) {
	ts := fmt.Sprintf("[%s] ", time.Now().String())
	msg := fmt.Sprintf(format, v...)
	fmt.Printf("%s%s", ts, msg)
}

type srvCfgT struct {
	cpuNum    int
	u64       bool
	size      int
	timelimit time.Duration
}

var srvCfg *srvCfgT

func init() {
	srvCfg = &srvCfgT{}
}

//
func send64(testCh chan *srvCfgT, inCh chan int64, closed chan struct{}) {
	counter := int64(0)
	for {
		select {
		case <-closed:
			inCh <- counter
			return
		default:
		}
		testCh <- srvCfg
		counter++
	}
}

//
func recv64(testCh chan *srvCfgT, outCh chan int64, closed chan struct{}) {
	counter := int64(0)
	for _ = range testCh {
		counter++
	}
	outCh <- counter
}

func send(testCh chan *srvCfgT, inCh chan int64, closed chan struct{}) {
	counter := int(0)
	for {
		select {
		case <-closed:
			inCh <- int64(counter)
			return
		default:
		}
		testCh <- srvCfg
		counter++
	}
}

func recv(testCh chan *srvCfgT, outCh chan int64, closed chan struct{}) {
	counter := int(0)
	for _ = range testCh {
		counter++
	}
	outCh <- int64(counter)
}
func main() {
	//
	preinit.SetPrefix("simpletester")
	preinit.SetOption("-c", "1", "cpu number")
	preinit.SetOption("-s", "1024", "channel buffer size")
	preinit.SetOption("-t", "10", "test time limit seconds")
	preinit.SetFlag("-64", "use int64")
	//
	srvCfg.cpuNum = preinit.GetInt("-c")
	srvCfg.size = preinit.GetInt("-s")
	srvCfg.timelimit = time.Duration(preinit.GetInt("-t")) * 1e9
	if srvCfg.timelimit <= 0 {
		srvCfg.timelimit = 10
	}
	srvCfg.u64 = preinit.GetFlag("-64")
	//
	srvCfg.cpuNum = preinit.SetGoMaxCPUs(srvCfg.cpuNum)
	//
	//preinit.Usage()
	//
	TimePrintf("Using %d/%d Cpus, using int64 %v, buffer size %d, time limit %v\n", srvCfg.cpuNum, runtime.NumCPU(), srvCfg.u64, srvCfg.size, srvCfg.timelimit)

	testCh := make(chan *srvCfgT, srvCfg.size)
	inCh := make(chan int64, 16)
	outCh := make(chan int64, 16)

	closed := make(chan struct{})

	starts := time.Now()
	if srvCfg.u64 {
		go send64(testCh, inCh, closed)
		go recv64(testCh, outCh, closed)
	} else {
		go send(testCh, inCh, closed)
		go recv(testCh, outCh, closed)
	}
	TimePrintf(" ... \n")
	tk := time.NewTicker(srvCfg.timelimit)
	defer tk.Stop()
	<-tk.C
	close(closed)
	intotal := <-inCh
	close(testCh)
	outtotal := <-outCh
	espts := time.Now().Sub(starts)
	TimePrintf("total, in %d(%f/s), out %d(%f/s), lost %d\n", intotal, float64(intotal*1e5)/float64(espts), outtotal, float64(outtotal*1e5)/float64(espts), intotal-outtotal)
}
