//

// open all file in directory and search one string,
// all file is format in line
package main

import (
	"bufio"
	"bytes"
	"os"
	"strings"
	"sync"

	"github.com/wheelcomplex/preinit/getopt"
	"github.com/wheelcomplex/preinit/misc"
	"github.com/wheelcomplex/preinit/palloc"
)

var opt = getopt.Opt
var tpf = misc.Tpf
var tpErrf = misc.TpErrf

//
type fileinfo struct {
	name string
	done bool
}

//
type searchinfo struct {
	bufile *bufio.Reader
	prefix []byte
	sep    []byte
}

// search in bufio.Reader
// TODO: signal handle
// TODO: search in one big file using multi cpus
// TODO: use mmap
func bufsearch(inCh chan *searchinfo, outCh chan []byte, wg *sync.WaitGroup) {
	defer wg.Done()

	alloc := palloc.NewAlloc(16)

	for info := range inCh {
		for {
			line, err := info.bufile.ReadBytes('\n')
			if err != nil {
				break
			}
			if bytes.Index(line, info.sep) == -1 {
				// mismatch
				continue
			}
			if len(info.prefix) > 0 {
				msg := alloc.Get(len(info.prefix) + len(line))
				copy(msg, info.prefix)
				copy(msg[len(info.prefix):], line)
				outCh <- msg
				alloc.Put(line)
			} else {
				outCh <- line
			}

		}
	}
	return
}

func main() {
	files := opt.OptVarStringList("-f/--file", "", "search file or directory list, default in current direcroty")
	sep := opt.OptVarString("-s/--search", "", "string search for")
	help := opt.OptVarBool("-h/--help", "false", "show this help message")
	if help || len(sep) == 0 || len(files) == 0 {
		opt.Usage()
		os.Exit(1)
	}
	tpf("ARGS: %v\n", os.Args)
	for idx, _ := range os.Args {
		tpf("\t%d: %v\n", idx, os.Args[idx])
	}
	ncpu := misc.GoMaxCPUs(-1)
	tpf("using %d cpus, searching string \"%s\" in list(%d): %v\n", ncpu, sep, len(files), files)

	inCh := make(chan *searchinfo, ncpu*4096)
	outCh := make(chan []byte, ncpu*4096)

	go func() {
		alloc := palloc.NewAlloc(16)
		bufout := bufio.NewWriter(os.Stdout)
		for msg := range outCh {
			bufout.Write(msg)
			alloc.Put(msg)
		}
	}()

	allfiles := make(map[string]*fileinfo, len(files))
	for _, onefile := range files {
		allfiles[onefile] = &fileinfo{
			name: onefile,
		}
	}
	newfilecnt := len(allfiles)
	idx := 0
	var wg sync.WaitGroup

	for newfilecnt > 0 {
		newfilecnt = 0
		for onefile, finfo := range allfiles {
			if finfo.done {
				continue
			}
			finfo.done = true
			newfilecnt++
			fd, err := os.Open(onefile)
			if err != nil {
				tpErrf("ERROR: %v\n", err.Error())
				continue
			}
			dirfiles, direrr := fd.Readdirnames(-1)
			if direrr != nil {
				if strings.HasSuffix(direrr.Error(), "not a directory") == false {
					// it's a dir but read failed
					tpErrf("ERROR: %s, %v\n", onefile, direrr.Error())
					continue
				}
				// it's a file, go ahead
			} else {
				// it's a dir
				for _, onefile := range dirfiles {
					allfiles[onefile] = &fileinfo{
						name: onefile,
					}
				}
				continue
			}
			bufile := bufio.NewReader(fd)
			if idx < ncpu {
				wg.Add(1)
				go bufsearch(inCh, outCh, &wg)
			}
			prefix := ""
			if len(files) > 1 {
				prefix = fd.Name() + ":"
			}
			inCh <- &searchinfo{
				bufile: bufile,
				prefix: []byte(prefix),
				sep:    []byte(sep),
			}
			idx++
		}
	}
	close(inCh)
	wg.Wait()
	close(outCh)
	tpf("all done!\n")
}
