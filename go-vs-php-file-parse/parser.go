// go-vs-php-file-parse test

package main

import (
	"bufio"
	"crypto/md5"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

//
type stat struct {
	lines uint64
	bytes uint64
	sizes uint64
	done  bool
	err   error
}

// default logger
var l = log.New(os.Stderr, "go-file-parser ", log.LstdFlags)

func main() {

	// input file name
	var filename string

	// read buffer size
	var readbufsize int

	// help flag
	var help bool

	// generate input file
	var gen bool

	var force bool

	// input file lines
	var filelines uint64

	//
	var profile bool

	//
	var showstat bool

	flag.StringVar(&filename, "f", "", "input file name")

	flag.BoolVar(&help, "h", false, "show this help message")

	flag.IntVar(&readbufsize, "s", 65535, "read buffer size(>= 512)")

	flag.Uint64Var(&filelines, "line", 1, "file lines in million(1,000,000), 1 million used 47 MB disk spaces")

	flag.BoolVar(&force, "force", false, "force overwrite existed output file")

	flag.BoolVar(&gen, "gen", false, "generate input file")

	flag.BoolVar(&profile, "profile", false, "enable http profile")

	flag.BoolVar(&showstat, "stat", false, "show parser stat every 5 seconds")

	flag.Parse()
	if help {
		flag.Usage()
		os.Exit(0)
	}
	if filelines <= 0 {
		flag.Usage()
		l.Fatalln("invalid file line(should be greate then zero)", filelines)
	}

	if filename == "" {
		filename = strconv.FormatInt(int64(filelines), 10) + "m.lines.testdata.txt"
	}

	// generate input file in consistent
	if gen {

		// bytes
		var seedBuf = []byte("31415926535897932384626433832795028841971693993751058209749445923078164062862089986280348253421170679821480865132823066470938446095505822317253594081284811174502841027019385211055596446229489549303819644288109756659334461284756482337867831652712019091456485669234603486104543266482133936072602491412737245870066063155881748815209209628292540917153643678925903600113305305488204665213841469519415116094330572703657595919530921861173819326117931051185480744623799627495673518857527248912279381830119491298336733624")

		totalline := filelines * 1000000
		filesize := time.Duration(totalline) * 50

		if force == false {
			if _, err := os.Stat(filename); err == nil {
				l.Fatalf("output file %s already existed, use flag -f for overwrite\n", filename)
			}
		}

		l.Printf("generating %s, %d lines, %d MB, be ware free disk spaces!\n", filename, totalline, filesize/1024/1024)

		fd, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			l.Fatalf("open/create output file %s failed: %s\n", filename, err.Error())
		}
		defer fd.Close()

		// 1M io buffer
		bufiofd := bufio.NewWriterSize(fd, 1024*1024)

		// LINE FORMAT(10 field split by space, end with return, 50 bytes):
		// 0123 1234 2345 3456 4567 5678 6789 7890 8901 9012\n

		if len(seedBuf) < 100 {
			panic("seedBuf too few")
		}

		linebuf := make([]byte, 50)
		// last byte never changed
		linebuf[49] = '\n'
		seekpos := 0
		starts := time.Now()
		maxpos := len(seedBuf) - 50
		h := md5.New()
		shift := 0
		maxshift := len(seedBuf) - 100
		for count := uint64(0); count < totalline; count++ {
			// for speedy, use copy and fill space
			copy(linebuf, seedBuf[seekpos:seekpos+49])
			for i := 4; i < 49; i = i + 5 {
				linebuf[i] = ' '
			}
			nn, err := bufiofd.Write(linebuf)
			if err != nil {
				l.Fatalf("write to output file %s failed: %s\n", filename, err.Error())
			}
			if nn < len(linebuf) {
				l.Fatalf("write to output file %s failed: short written\n", filename)
			}
			h.Write(linebuf)

			// move buffer position
			seekpos += 50
			if seekpos > maxpos {
				shift++
				if shift >= maxshift {
					shift = 0
				}
				seekpos = shift
			}
		}
		if err := bufiofd.Flush(); err != nil {
			l.Fatalf("flush output file %s failed: %s\n", filename, err.Error())
		}
		bufiofd.Reset(nil)
		l.Printf("waiting for disk flush ...\n")
		esp := time.Now().Sub(starts)
		if esp <= 0 {
			esp = 1
		}
		// to MB/s
		bw := filesize * time.Second / esp / 1024 / 1024
		if err := fd.Sync(); err != nil {
			l.Printf("WARNING: disk flush failed: %s\n", err.Error())
		}
		l.Printf("%s generated in %v, MD5SUM %s, %d lines, %d MB, %d MB/s.\n", filename, esp, fmt.Sprintf("%x", h.Sum(nil)), totalline, filesize/1024/1024, bw)
		return
	}

	if readbufsize < 512 {
		l.Printf("invalid read buffer size %d\n", readbufsize)
		flag.Usage()
		os.Exit(0)
	}

	filestat, err := os.Stat(filename)
	if err != nil {
		l.Fatalf("stat input file %s failed: %s\n", filename, err)
	}
	if filestat.IsDir() {
		l.Fatalf("input file %s is directory.\n", filename)
	}

	//
	fd, err := os.Open(filename)
	if err != nil {
		l.Fatalf("open input file %s for read failed: %s\n", filename, err)
	}
	defer fd.Close()

	bufiofd := bufio.NewReaderSize(fd, readbufsize)

	// enable profile
	if profile {
		hostPort := "127.0.0.1:6090"

		binpath, _ := filepath.Abs(os.Args[0])
		fmt.Printf("\n http profile:\n")
		fmt.Printf("\n http://%s/debug/pprof/\n", hostPort)
		fmt.Printf("\n http://%s/debug/pprof/goroutine?debug=1\n\n", hostPort)
		fmt.Printf("\nCPU profile: go tool pprof %s http://%s/debug/pprof/profile\n\n", binpath, hostPort)
		fmt.Printf("\nMEM profile:  go tool pprof %s http://%s/debug/pprof/heap\n\n", binpath, hostPort)
		server := &http.Server{Addr: hostPort, Handler: nil}
		ln, err := net.Listen("tcp", hostPort)
		if err != nil {
			l.Fatalf("GoHttpProfile initial failed, %s", err.Error())
		}
		go server.Serve(ln)
		defer ln.Close()
	}

	l.Printf("Go, parsing %s(%d MB), read buffer size %d ...\n", filename, filestat.Size()/1024/1024, readbufsize)

	var wg sync.WaitGroup

	// big chan size for not blocking parser
	statCh := make(chan *stat, 8192)
	wg.Add(1)
	go show(statCh, l, &wg)
	// wait for show initial
	time.Sleep(5 * time.Microsecond)

	var result *stat

	if showstat {
		result = parser(bufiofd, statCh, 5)
	} else {
		result = parser(bufiofd, nil, 5)
	}
	if result.err != nil {
		l.Fatalf("parse failed: %s\n", result.err)
	}
	bufiofd.Reset(nil)
	statCh <- result
	wg.Wait()
	// wait for last stat
	return
}

//
func show(statCh chan *stat, l *log.Logger, wg *sync.WaitGroup) {
	defer wg.Done()
	startts := time.Now()
	prets := time.Now()
	nowts := time.Now()
	var prebytes, totalbw, curbw uint64
	var secMutil uint64 = uint64(time.Second) / 1048576
	for {
		result := <-statCh
		nowts = time.Now()
		esp := nowts.Sub(prets)
		totalesp := nowts.Sub(startts)
		if esp <= 0 {
			esp = 1
		}
		if totalesp <= 0 {
			totalesp = 1
		}
		// overflow check
		if result.bytes > uint64(totalesp) {
			totalbw = secMutil * (result.bytes / uint64(totalesp))
		} else {
			totalbw = (secMutil * result.bytes) / uint64(totalesp)
		}
		curbw = (secMutil * (result.bytes - prebytes)) / uint64(esp)
		if result.done {
			// parser is done
			l.Printf("total time esp %v(%v), %d lines, %d(%dMB), %d(%d) MB/s.\n", totalesp, esp, result.lines, result.bytes, result.bytes/1024/1024, totalbw, curbw)
			return
		} else {
			l.Printf("      time esp %v(%v), %d lines, %d(%dMB), %d(%d) MB/s.\n", totalesp, esp, result.lines, result.bytes, result.bytes/1024/1024, totalbw, curbw)
		}
		prets = nowts
		prebytes = result.bytes
	}
}

// AtoiString a simple Atoi() function for base 10 only,
// invalid slice will convert to zero
func AtoIntOfBytes(s []byte) (int, error) {
	var res int = 0
	var isnav bool

	for idx, char := range s {
		if idx == 0 && char == '-' {
			isnav = true
			continue
		}
		if char < '0' || char > '9' {
			return 0, errors.New("invalid number slice " + string(s))
		}
		res = res*10 + int(char) - '0'
	}
	if isnav {
		res = -res
	}
	return res, nil
}

// SplitBytes slices s into subslices separated by sep and save into result a slice of
// the subslices between those separators.
// Will extend if result have not enough space to hold return [][]byte.
// The count determines the number of subslices to return:
//   n > 0: at most n subslices; the last subslice will be the unsplit remainder.
//   n == 0: the result is empty [][]byte
//   n < 0: all subslices
// warning, if not bytes exist befor sep, subslice will be []byte{}.
//
func SplitBytes(s []byte, sep byte, n int, result [][]byte) [][]byte {
	if n == 0 {
		return result
	}
	if len(result) > 0 {
		result = result[:0]
	}
	if n > 0 {
		n--
	}
	rcnt := 0
	pos := 0
	for idx, char := range s {
		if char == sep {
			rcnt++
			if n > 0 && rcnt > n {
				result = append(result, s[pos:])
				pos = len(s)
				break
			}
			result = append(result, s[pos:idx])
			pos = idx + 1
		}
	}
	if pos < len(s) {
		result = append(result, s[pos:])
	}
	return result
}

// logic from http://www.oschina.net/question/938918_2145778?fromerr=ZAS07vf6
func parser(bfRd *bufio.Reader, statCh chan *stat, interval int) *stat {
	var line []byte
	var slice [][]byte
	var err error

	result := &stat{}

	if interval <= 0 {
		interval = 5
	}
	tk := time.NewTicker(time.Duration(interval) * time.Second)
	defer tk.Stop()

	for {
		line, err = bfRd.ReadSlice('\n')
		if err != nil {
			if err == io.EOF {
				//l.Printf("read done, %d lines, %d bytes, parse size %d\n", result.lines, result.bytes, result.sizes)
				err = nil
			} else {
				l.Fatalf("read failed: %s\n", err.Error())
			}
			result.err = err
			result.done = true
			return result
		}
		result.bytes += uint64(len(line))
		result.lines++
		slice = SplitBytes(line, ' ', 10, slice)
		if len(slice) < 9 {
			l.Printf("invalid line, splited less then 9(%d): %s\n", len(slice), slice)
			continue
		}
		size, err := AtoIntOfBytes(slice[8])
		if err != nil {
			l.Printf("warning: %s\n", err.Error())
			size = 0
		}
		result.sizes += uint64(size)
		if statCh != nil {
			// send stat
			select {
			case <-tk.C:
				statCh <- result
			default:
			}
		}
	}
}

//
