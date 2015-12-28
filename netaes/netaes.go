//
// net aes test
//
//
package main

import (
	"bytes"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/wheelcomplex/preinit/cmtp"
	"github.com/wheelcomplex/preinit/keyaes"
	"github.com/wheelcomplex/preinit/murmur3"
)

//var key = []byte("0123456789-0-")
//var key = []byte("--CMTP--")
var key = []byte("")

func main() {
	profileport := "6060"
	profileurl := "localhost:" + profileport
	fmt.Printf("profile: http://localhost:%s/debug/pprof/\n", profileport)
	go func() {
		// http://localhost:6060/debug/pprof/
		fmt.Println(http.ListenAndServe(profileurl, nil))
	}()
	var err error
	ae := keyaes.NewAES(key, murmur3.New32())
	//plaintext := []byte("---6c35b38c8346d455744d53afb9dfsdfsdfa883194879cd69---")
	// 1MB
	plaintext := make([]byte, 1024*1024)
	entxt := make([]byte, ae.EncryptSize(len(plaintext)))
	entxt = ae.Encrypt(entxt, plaintext)
	//fmt.Printf("P(%d):%s\n", len(plaintext), plaintext)
	//fmt.Printf("E(%d):%x %x %x\n", len(entxt), entxt[:8], entxt[8:16], entxt[16:])
	//entxt[0] = 'F'
	//entxt[15] = 0xf
	//fmt.Printf("E(%d):%x %x %x\n", len(entxt), entxt[:8], entxt[8:16], entxt[16:])
	detxt := make([]byte, len(entxt))
	detxt, err = ae.Decrypt(detxt, entxt)
	if err != nil {
		fmt.Printf("Dencrypt failed: %v\n", err)
		os.Exit(8)
	}
	//fmt.Printf("P(%d):%x\n", len(plaintext), plaintext)
	//fmt.Printf("D(%d):%s\n", len(detxt), detxt)
	//fmt.Printf("E(%d):%x %x %x\n", len(entxt), entxt[:8], entxt[8:16], entxt[16:])
	if bytes.Equal(detxt, plaintext) == false {
		fmt.Printf("Dencrypt failed: %v\n", "mismatch")
		os.Exit(8)
	}
	startts := time.Now()
	runtime.GOMAXPROCS(runtime.NumCPU())
	wg := sync.WaitGroup{}
	loop := int(1e4)
	for i := 0; i < runtime.NumCPU()-1; i++ {
		wg.Add(1)
		go func(nr int) {
			defer wg.Done()
			plaintext := make([]byte, 1024*1024)
			//detxt := make([]byte, len(entxt))
			ae := keyaes.NewAES(key, cmtp.NewNoopChecksum())
			ae = keyaes.NewAES(key, murmur3.New32())
			//ae = keyaes.NewAES(key, xxhash.New(0))
			entxt := make([]byte, ae.EncryptSize(len(plaintext)))
			entxt = ae.Encrypt(entxt, plaintext)
			for i := 0; i < loop; i++ {
				//entxt = ae.Encrypt(entxt, plaintext)
				ae.Encrypt(entxt, plaintext)
				//ae.Decrypt(detxt, entxt)
				//fmt.Printf("E(%d):%x %x %x\n", len(entxt), entxt[:8], entxt[8:16], entxt[16:])
			}
			fmt.Printf("#%d exited\n", nr)
		}(i)
	}
	wg.Wait()
	//fmt.Printf("E(%d):%x %x %x\n", len(entxt), entxt[:8], entxt[8:16], entxt[16:])
	esp := time.Now().Sub(startts)
	total := loop*runtime.NumCPU() - 1
	tqps := float32(total) / float32(esp.Seconds())
	pqps := tqps / float32(runtime.NumCPU()-1)
	fmt.Printf("COUNT %d, ESP %v, QPS %f(%f)\n", loop, esp, tqps, pqps)
	//time.Sleep(10e9)
}
