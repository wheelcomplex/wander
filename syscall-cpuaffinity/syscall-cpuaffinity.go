package main

import (
	"fmt"
	"math"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/wheelcomplex/preinit/misc"
)

var masterExiting = make(chan struct{}, 0)

func main() {
	//
	profileport := "6090"
	profileurl := "localhost:" + profileport
	fmt.Printf("profile: [http://localhost:%s/debug/pprof/]\n", profileport)
	fmt.Printf("go tool pprof %s http://localhost:%s/debug/pprof/profile\n", os.Args[0], profileport)
	go func() {
		// http://localhost:6060/debug/pprof/
		fmt.Println(http.ListenAndServe(profileurl, nil))
	}()
	//
	ident := "[master#" + strconv.Itoa(os.Getpid()) + "]"
	//
	// enable all CPUs
	misc.SetGoMaxCPUs(-1)
	//
	var exitNotifyChan = make(chan os.Signal, 128)
	var sigChan = make(chan os.Signal, 128)
	//
	//all incoming signals will be catched
	signal.Notify(sigChan)
	notifys := []os.Signal{syscall.SIGCHLD}
	exits := []os.Signal{syscall.SIGABRT, syscall.SIGINT, syscall.SIGPIPE, syscall.SIGQUIT, syscall.SIGTERM}
	go sigHandle(ident, sigChan, exitNotifyChan, notifys, exits)

	//runtime.LockOSThread()
	//
	for i := 0; i < 3; i++ {
		go stressCPU()
	}
	//
	showAffinity("initial")
	//
	pid := os.Getpid()
	err := syscall.SetAffinity(uintptr(pid), []int{0})
	if err != nil {
		fmt.Printf("SetAffinity(%d): %v\n", pid, err)
		os.Exit(1)
	}
	//
	tids, err := os.GetThreadIDs(pid)
	if err != nil {
		fmt.Printf("GetThreadID(%d): %v\n", pid, err)
		os.Exit(1)
	}
	fmt.Printf("Thread ID(%d): \n", pid)
	for _, tid := range tids {
		if tid == pid {
			continue
		}
		err := syscall.SetAffinity(uintptr(tid), []int{1, 2, 3})
		if err != nil {
			fmt.Printf("SetAffinity(%d/%d): %v\n", tid, pid, err)
			os.Exit(1)
		}
	}
	showAffinity("set al thread to 1,2,3")
	time.Sleep(20 * time.Second)
	for _, tid := range tids {
		if tid == pid {
			continue
		}
		err := syscall.SetAffinity(uintptr(tid), []int{4, 5, 6})
		if err != nil {
			fmt.Printf("SetAffinity(%d/%d): %v\n", tid, pid, err)
			os.Exit(1)
		}
	}
	showAffinity("set al thread to 4,5,6")

	// waiting for signal
	<-exitNotifyChan
	signal.Ignore()
	close(masterExiting)
	//

	fmt.Printf(ident + " end\n")
}

//
func showAffinity(msg string) {
	println("---", msg, "---")
	//
	pid := os.Getpid()
	upid := uintptr(pid)
	var cpus []int
	var err error

	//
	cpus, err = syscall.GetAffinity(upid)
	if err != nil {
		fmt.Printf("GetAffinity(%d): %v\n", pid, err)
		os.Exit(1)
	}
	fmt.Printf("GetAffinity(%d): %d, %v\n", pid, len(cpus), cpus)
	tids, err := os.GetThreadIDs(pid)
	if err != nil {
		fmt.Printf("GetThreadID(%d): %v\n", pid, err)
		os.Exit(1)
	}
	fmt.Printf("Thread ID(%d): \n", pid)
	for idx, tid := range tids {
		cpus, err = syscall.GetAffinity(uintptr(tid))
		if err != nil {
			fmt.Printf("GetAffinity(%d): %v\n", tid, err)
			os.Exit(1)
		}
		fmt.Printf("\t#%d\t%d\t%d, %v\n", idx, tid, len(cpus), cpus)
	}
	println("------")
}

// sigHandle
func sigHandle(tag string, sigChan, exitNotifyChan chan os.Signal, notifys []os.Signal, exits []os.Signal) {
	exiting := false
	for {
		sig := <-sigChan
		notifyed := false
		if len(notifys) > 0 {
			for _, val := range notifys {
				if val == sig {
					exitNotifyChan <- sig
					if len(tag) > 0 {
						fmt.Printf("%s notifyed signal: %d/%s\n", tag, sig, sig.String())
						notifyed = true
					}
					break
				}
			}
		}
		if exiting == false && len(exits) > 0 {
			for _, val := range exits {
				if val == sig {
					exiting = true
					exitNotifyChan <- sig
					fmt.Printf("\n%s send exiting signal: %d/%s\n", tag, sig, sig.String())
					break
				}
			}
			if exiting == false && len(tag) > 0 && notifyed == false {
				fmt.Printf("\n%s exiting signal ignored: %d/%s\n", tag, sig, sig.String())
			}
		} else if len(tag) > 0 && notifyed == false {
			fmt.Printf("%s ignored signal: %d/%s\n", tag, sig, sig.String())
		}
	}
}

//
func stressCPU() {
	//	runtime.LockOSThread()
	var count uint64 = math.MaxUint64
	var divcount uint64
	for {
		select {
		case <-masterExiting:
			return
		default:
		}
		//runtime.Gosched()
		divcount = count / 5
		count--
		divcount++
		if count == 0 {
			count = math.MaxUint64
		}
	}
}

//
