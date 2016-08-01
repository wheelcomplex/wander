package main

/*
#include <stddef.h>
#include <time.h>
#include <pthread.h>

#define THREADS (50)

extern void GoFn();

static void* thread(void* darg) {
    struct timespec ts;

    int *arg = (int*)(darg);
    ts.tv_sec = 0;
    ts.tv_nsec = 1000 * (THREADS - *arg);
    nanosleep(&ts, NULL);

    GoFn();

    return NULL;
}

static void CFn() {
    int i;
    pthread_t tids[THREADS];

    for (i = 0; i < THREADS; i++) {
        pthread_create(&tids[i], NULL, thread, (void*)(&i));
    }
    for (i = 0; i < THREADS; i++) {
        pthread_join(tids[i], NULL);
    }
}
*/
import "C"

import (
	"bytes"
	"fmt"
	"os/exec"
	"time"
)

//export GoFn
func GoFn() {
	time.Sleep(time.Millisecond)
}

func main() {

	go cmdTimer()
	ticker := time.NewTicker(time.Millisecond * 1000)
	for _ = range ticker.C {
		C.CFn()
	}
}

func cmdTimer() {
	t := time.Now()

	cmdRun := func(id string) {
		cmd := exec.Command("iostat")
		cmd.Dir = "/"
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Run()
		fmt.Println(t.String() + " " + id)
	}

	ticker := time.NewTicker(time.Millisecond * 500)
	for _ = range ticker.C {
		go cmdRun("1")
		go cmdRun("2")
		go cmdRun("3")
		go cmdRun("4")
	}

}
