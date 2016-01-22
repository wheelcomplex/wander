//

//
package main

import (
	"fmt"
	"math"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/wheelcomplex/preinit/getopt"
	"github.com/wheelcomplex/preinit/misc"
)

var opt = getopt.Opt
var masterExiting = make(chan struct{}, 0)

type workerStatT struct {
	index   int  //
	pid     int  //
	stopped bool //
	warned  bool //
	proc    *exec.Cmd
}

func main() {
	// worker enter
	procState := opt.GetString("--ident")
	if strings.HasPrefix(procState, "worker#") {
		stressWorker()
		return
	}
	var workerNum int
	var showhelp bool
	opt.OptBool(&showhelp, "--help/-h", "false", "show help")
	opt.OptInt(&workerNum, "-w", "0", "number of workers, 0 for NumCPUs - 1, at less 1")

	if showhelp {
		opt.Usage()
		return
	}
	profileport := "6090"
	profileurl := "localhost:" + profileport
	fmt.Printf("profile: [http://localhost:%s/debug/pprof/]\n", profileport)
	fmt.Printf("go tool pprof %s http://localhost:%s/debug/pprof/profile\n", os.Args[0], profileport)
	go func() {
		// http://localhost:6060/debug/pprof/
		fmt.Println(http.ListenAndServe(profileurl, nil))
	}()
	// reserve one cpu for master
	if workerNum < 0 {
		workerNum = 0
	}
	workerNum = misc.MaxNumWorker(workerNum)
	//
	ident := "[master#" + strconv.Itoa(os.Getpid()) + "]"
	//
	// enable all CPUs
	misc.SetGoMaxCPUs(-1)
	// pin myselft to last cpu
	tids, _ := os.GetThreadIDs(os.Getpid())
	for _, val := range tids {
		err := syscall.SetAffinity(uintptr(val), []int{misc.GoMaxCPUs(0)})
		if err != nil {
			fmt.Printf(ident+" SetAffinity(%d): %v, %v\n", os.Getpid(), []int{misc.GoMaxCPUs(0)}, err)
		}
	}
	var exitNotifyChan = make(chan os.Signal, 128)
	var sigChan = make(chan os.Signal, 128)
	//
	//all incoming signals will be catched
	signal.Notify(sigChan)
	notifys := []os.Signal{syscall.SIGCHLD}
	exits := []os.Signal{syscall.SIGABRT, syscall.SIGINT, syscall.SIGPIPE, syscall.SIGQUIT, syscall.SIGTERM}
	go sigHandle(ident, sigChan, exitNotifyChan, notifys, exits)

	// fork worker
	pidList := make(map[int]*workerStatT)
	fmt.Printf(ident+" %d CPUs avaible, launch %d worker ...\n", misc.SetGoMaxCPUs(-1), workerNum)
	for i := 0; i < workerNum; i++ {
		proc, err := stressFork(i, os.Getpid())
		if err != nil {
			fmt.Printf(ident+" start worker#%d: %s\n", i, err)
			continue
		}
		pidList[i] = &workerStatT{
			index: i,
			pid:   proc.Process.Pid,
			proc:  proc,
		}
		//fmt.Printf(ident+" started worker#%d: %d\n", i, proc.Process.Pid)
		time.Sleep(500 * time.Microsecond)
	}

	if len(pidList) == 0 {
		fmt.Printf(ident+" all %d worker fail to start, exit.\n", workerNum)
	} else {

		go misc.WaitChild(-1)
		time.Sleep(1000 * time.Microsecond)
		alivecnt := stressCount(pidList, 500*time.Microsecond)
		fmt.Printf(ident+" %d/%d workers running ...\n", alivecnt, len(pidList))

		// go stressAffinity(ident, pidList)

		// waiting for signal
		stopcnt := 0
		for {
			sig := <-exitNotifyChan
			if sig == syscall.SIGCHLD {
				stopcnt++
				if stopcnt >= len(pidList) {
					fmt.Printf("all worker(%d/%d) stopped(unexcepted).\n", stopcnt, len(pidList))
					break
				}
			} else {
				//fmt.Printf(ident+" killing worker for signal: %s\n", sig.String())
				misc.UNUSED(sig)
				break
			}
		}
		close(masterExiting)
		//
		signal.Ignore()
		//
		err := stressWait(pidList)
		if err != nil {
			fmt.Printf(ident+" error: %s\n", err.Error())
		}
	}
	//

	fmt.Printf(ident + " end\n")
}

//
func stressAffinity(ident string, pidList map[int]*workerStatT) error {
	// len(pidList) <= (runtime.NumCPU() - 1)
	tk := time.NewTicker(5000 * time.Microsecond)
	defer tk.Stop()
	for {
		select {
		case <-masterExiting:
			return nil
		default:
		}
		for _, stat := range pidList {
			if stat.warned {
				continue
			}
			pid := stat.pid
			idx := stat.index
			cpuIndex := (idx % runtime.NumCPU())
			tids, _ := os.GetThreadIDs(pid)
			for _, val := range tids {
				err := syscall.SetAffinity(uintptr(val), []int{cpuIndex})
				if err != nil {
					fmt.Printf(ident+" SetAffinity(%d, %d(%d): %v, %v\n", idx, pid, val, []int{cpuIndex}, err)
				} else {
					//fmt.Printf(ident+" SetAffinity(%d, %d(%d): %v, %v\n", idx, pid, val, []int{cpuIndex}, err)
				}
			}
		}
		for _, stat := range pidList {
			if stat.warned == false && misc.IsPidDie(stat.pid) {
				stat.warned = true
				fmt.Printf("unexcepted worker(%d/%d) stopped.\n", stat.pid, len(pidList))
			}
		}
		<-tk.C
	}
	return nil
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

// stressWait kill and wait for worker exit
func stressWait(pidList map[int]*workerStatT) error {
	// timeout
	tm := time.NewTimer(5 * time.Second)
	defer tm.Stop()
	for _, stat := range pidList {
		syscall.Kill(stat.pid, syscall.SIGTERM)
		//fmt.Printf("[stressWait] killing worker: %d/%d.\n", stat.pid, len(pidList))
	}
	aliveChan := make(chan int, len(pidList))
	go func() {
		// check for stop
		alivecnt := len(pidList)
		for alivecnt > 0 {
			for _, stat := range pidList {
				if stat.stopped == false && misc.IsPidDie(stat.pid) {
					stat.stopped = true
					alivecnt--
					aliveChan <- stat.pid
				}
			}
		}
		//fmt.Printf("[stressWait] all worker stopped.\n")
	}()
	stopcnt := 0
	for {
		select {
		case <-tm.C:
			for _, stat := range pidList {
				syscall.Kill(stat.pid, syscall.SIGKILL)
			}
			return fmt.Errorf("worker KILLED for stop timeout\n")
		case pid := <-aliveChan:
			// fmt.Printf("worker(%d) stopped.\n", pid)
			misc.UNUSED(pid)
			stopcnt++
			if stopcnt >= len(pidList) {
				return nil
			}
		}
	}
}

//
func stressCount(pidList map[int]*workerStatT, wait time.Duration) int {
	alivecnt := 0
	tk := time.NewTicker(wait)
	defer tk.Stop()
	for {
		alivecnt = 0
		for _, stat := range pidList {
			if misc.IsPidAlive(stat.pid) {
				alivecnt++
			}
		}
		if alivecnt >= len(pidList) {
			return alivecnt
		}
		select {
		case <-tk.C:
			return alivecnt
		default:
			time.Sleep(200 * time.Microsecond)
		}
	}
	return alivecnt
}

//
// stressFork create child and return pid
func stressFork(idx int, masterpid int) (*exec.Cmd, error) {
	opt := getopt.NewDefaultOpts()
	opt.ModifySetOption("--ident", "worker#"+strconv.Itoa(idx))
	opt.ModifySetOption("--ppid", strconv.Itoa(masterpid))
	//
	opt.ModifyDelOption("-n", "")
	cmdPath, ferr := filepath.Abs(os.Args[0])
	if ferr != nil {
		fmt.Printf("worker#%d probe execfile: %s\n", idx, ferr.Error())
		return nil, ferr
	}
	fullArgs := []string{cmdPath}
	fullArgs = append(fullArgs, opt.StringCmdListOrig()...)
	cpuIndex := (idx % runtime.NumCPU())
	//fmt.Printf("worker#%d cmd: %s %s || %v\n", idx, cmdPath, opt.StringCmdLineOrig(), fullArgs)
	sysAttr := &syscall.SysProcAttr{
		Affinitys:  []int{cpuIndex},
		Chroot:     "",
		Setsid:     true,
		Noctty:     false,
		Foreground: false,
		Pdeathsig:  syscall.SIGTERM, //  Signal that the process will get when its parent dies (Linux only)
	}
	cmd := &exec.Cmd{
		Path:        cmdPath,
		Dir:         "/tmp/",
		Args:        fullArgs,
		Stdin:       nil,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		SysProcAttr: sysAttr,
	}
	err := cmd.Start()
	return cmd, err
}

// stressWorker stress CPU in dead loop,
func stressWorker() {
	opt := getopt.NewDefaultOpts()
	ident := opt.GetString("--ident")
	misc.SetGoMaxCPUs(1)
	if len(ident) == 0 {
		ident = "worker#999"
	}
	ident = "[" + ident + "]"

	masterPID := opt.GetInt("--ppid")
	if masterPID == 0 {
		masterPID = os.Getppid()
	}
	fmt.Printf(ident+" %d CPUs avaible ...\n", misc.SetGoMaxCPUs(-1))
	showAffinity(ident)
	var count uint64 = math.MaxUint64
	var divcount uint64
	// TODO: notify to master
	for {
		//runtime.Gosched()
		divcount = count / 5
		count--
		divcount++
		if count == 0 {
			count = math.MaxUint64
		}
		if divcount%5e8 == 0 {
			if misc.IsPidDie(masterPID) {
				// exit when masterPID die
				return
			}
		}
	}
	os.Exit(0)
}

//

// sigHandle
func sigHandle(tag string, sigChan, exitNotifyChan chan os.Signal, notifys []os.Signal, exits []os.Signal) {
	exiting := false
	for {
		sig := <-sigChan
		notifyed := false
		for _, val := range notifys {
			if val == sig {
				exitNotifyChan <- sig
				if len(tag) > 0 {
					fmt.Printf("\nsignal handle of %s, notifyed signal: %d/%s\n", tag, sig, sig.String())
					notifyed = true
				}
				break
			}
		}
		if exiting == false {
			for _, val := range exits {
				if val == sig {
					exiting = true
					exitNotifyChan <- sig
					fmt.Printf("\nsignal handle of %s, sant exiting signal: %d/%s\n", tag, sig, sig.String())
					break
				}
			}
			if exiting == false && len(tag) > 0 && notifyed == false {
				fmt.Printf("\nsignal handle of %s, exiting signal ignored: %d/%s\n", tag, sig, sig.String())
			}
		} else if len(tag) > 0 && notifyed == false {
			fmt.Printf("\nsignal handle of %s, ignored signal: %d/%s\n", tag, sig, sig.String())
		}
	}
}

//
