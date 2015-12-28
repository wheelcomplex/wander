package main

import (
	"flag"
	"runtime"
	"time"
)

type res struct {
	id    int
	count int
}

func worker(i int, c chan struct{}, resCh chan res) {
	var totalin int
	for _ = range c {
		totalin++
	}
	resCh <- res{i, totalin}
}

var argNrProcs = flag.Int("n", 8, "GOMAXPROCS")
var argLoop = flag.Int("l", 8, "loop times")
var nworker = flag.Int("w", 8, "workers")

func main() {
	flag.Parse()
	if *argNrProcs > 0 {
		runtime.GOMAXPROCS(*argNrProcs)
	}
	countch := make(chan res, *nworker)
	winner := make(map[int]int, *nworker)
	loopcnt := 0
	loopmax := *nworker * 1e5
	println("total threads:", runtime.GOMAXPROCS(-1))
	println("total loop:", *argLoop)
	println("total send:", loopmax)
	println("total worker:", *nworker)
	for i := 0; i < *nworker; i++ {
		winner[i] = 0
	}
	for {
		loopcnt++
		println("------")
		println("loop:", loopcnt)
		c := make(chan struct{}, loopmax)
		for i := 0; i < *nworker; i++ {
			go worker(i, c, countch)
		}
		for i := 0; i < loopmax; i++ {
			c <- struct{}{}
		}
		time.Sleep(1e8)
		close(c)
		for i := 0; i < *nworker; i++ {
			res := <-countch
			winner[res.id] += res.count
			println(res.id, res.count)
		}
		println(" -")
		if loopcnt >= *argLoop {
			break
		}
	}
	println("total threads:", runtime.GOMAXPROCS(-1))
	println("total loop:", *argLoop)
	println("total send:", loopmax)
	println("total worker:", *nworker)
	println("final count:")
	for i := 0; i < *nworker; i++ {
		println(i, winner[i])
	}
}
