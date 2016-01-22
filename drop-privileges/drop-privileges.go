package main

import (
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

var worker bool
var help bool

func main() {
	flag.BoolVar(&worker, "worker", false, "run in worker mode, should be used by master")
	flag.BoolVar(&help, "h", false, "show help")
	flag.Parse()
	if help {
		flag.PrintDefaults()
		os.Exit(0)
	}

	var sigChan = make(chan os.Signal, 128)
	var exitNotifyChan = make(chan os.Signal, 128)

	notifys := []os.Signal{syscall.SIGCHLD}
	exits := []os.Signal{syscall.SIGABRT, syscall.SIGINT, syscall.SIGPIPE, syscall.SIGQUIT, syscall.SIGTERM}

	if worker == false {
		if os.Getuid() != 0 {
			fmt.Println("master mode must run by UID 0(user root)")
			os.Exit(1)
		}
		fmt.Println("Dropping privileges...")
		cmd, err := drop()
		if err != nil {
			fmt.Println("Failed to drop privileges:", err)
			os.Exit(1)
		}
		pid := cmd.Process.Pid

		//all incoming signals will be catched
		signal.Notify(sigChan)
		go sigHandle("master", sigChan, exitNotifyChan, notifys, exits)

		// signal handle
		go func(cmd *exec.Cmd) {
			sig := <-exitNotifyChan
			signal.Ignore()
			cmd.Process.Signal(sig)
			time.Sleep(1 * time.Second)
			fmt.Printf("master killing worker %d for signal: %s\n", pid, sig.String())
			cmd.Process.Kill()
			time.Sleep(5 * time.Second)
			cmd.Process.Signal(syscall.SIGKILL)
			return
		}(cmd)
		//

		cmd.Wait()

		fmt.Printf("worker %d exited\n", pid)

		os.Exit(0)
	} else {
		if os.Getuid() == 0 {
			fmt.Println("can not run http server with UID 0(user root)")
			os.Exit(1)
		}
	}

	l, err := net.FileListener(os.NewFile(3, "[socket]"))
	if err != nil {
		// Yell into the void.
		fmt.Println("http server failed to listen on FD 3:", err)
		os.Exit(1)
	}

	//all incoming signals will be catched
	signal.Notify(sigChan)
	go sigHandle("http server", sigChan, exitNotifyChan, notifys, exits)
	
	// signal handle
	go func() {
		sig := <-exitNotifyChan
		signal.Ignore()
		fmt.Printf("http server %d/%s exit for signal: %s\n", os.Getpid(), l.Addr().String(), sig.String())
		os.Exit(0)
	}()

	msg := fmt.Sprintf("http server listen at %s, process %d running as %d/%d\n", l.Addr().String(), os.Getpid(), os.Getuid(), os.Getgid())
	
	fmt.Println(msg)

	http.Serve(l, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, msg)
	}))
}

func drop() (*exec.Cmd, error) {
	l, err := net.Listen("tcp", ":80")
	if err != nil {
		return nil, err
	}

	f, err := l.(*net.TCPListener).File()
	if err != nil {
		return nil, err
	}
	fmt.Printf("master mode, http server listen at %s\n", l.Addr().String())

	cmd := exec.Command(os.Args[0], "-worker")
	cmd.ExtraFiles = []*os.File{f}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid: 65534,
			Gid: 65534,
		},
		Setsid: true,
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Println("capture stdout of worker failed:", err)
	} else {
		go func() {
			var totalbytes int64
			for {
				bytes, err := io.Copy(os.Stdout, stdout)
				if err != nil {
					fmt.Printf("stdout io.Copy %d bytes, exit with %s\n", totalbytes, err)
					break
				}
				totalbytes += bytes
			}
		}()
	}
	
	stderr, err := cmd.StderrPipe()
	if err != nil {
		fmt.Println("capture stderr of worker failed:", err)
	} else {
		go func() {
			var totalbytes int64
			for {
				bytes, err := io.Copy(os.Stderr, stderr)
				if err != nil {
					fmt.Printf("stderr io.Copy %d bytes, exit with %s\n", totalbytes, err)
					break
				}
				totalbytes += bytes
			}
		}()
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	fmt.Printf("Spawned process %d, wating ...\n", cmd.Process.Pid)
	return cmd, nil
}


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
