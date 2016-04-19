//

//
package tmp

/*
./bench.sh
 --- all
BenchmarkPrepare-2      	  100000	     75622 ns/op	       0 B/op	       0 allocs/op
BenchmarkMaprange-2     	     500	  18368377 ns/op	    8695 B/op	     205 allocs/op
BenchmarkMaplist-2      	     500	  13697910 ns/op	    8706 B/op	     205 allocs/op
BenchmarkSlicerange-2   	   30000	    292621 ns/op	      80 B/op	       3 allocs/op
BenchmarkSlicelist-2    	   30000	    295882 ns/op	      80 B/op	       3 allocs/op
PASS
ok  	_/home/rhinofly/home/tmp/aaa	51.467s
------
go tool pprof bin.out.all ./cpu.pprof.all
go tool pprof bin.out.all ./mem.pprof.all
------
*/

import "testing"

type wheelinfo struct {
	idx   uint64 // wheel index
	round uint64 // the round of this wheel
}

const max uint64 = 10e4

func TestSlicelist(t *testing.T) {
}

func BenchmarkPrepare(b *testing.B) {
	var cnt uint64
	for i := 0; i < b.N; i++ {
		for i := uint64(0); i < max; i++ {
			cnt++
		}
	}
}

func BenchmarkMaprange(b *testing.B) {

	bmap := mapinit()
	for i := 0; i < b.N; i++ {
		for i, _ := range bmap {
			_ = bmap[i].idx
		}
	}
}

func BenchmarkMaplist(b *testing.B) {

	bmap := mapinit()
	for i := 0; i < b.N; i++ {
		for i := uint64(0); i < max; i++ {
			_ = bmap[i].idx
		}
	}
}

func BenchmarkSlicerange(b *testing.B) {

	bslice := sliceinit()
	for i := 0; i < b.N; i++ {
		for i, _ := range bslice {
			_ = bslice[i].idx
		}
	}
}

func BenchmarkSlicelist(b *testing.B) {

	bslice := sliceinit()
	for i := 0; i < b.N; i++ {
		for i := uint64(0); i < max; i++ {
			_ = bslice[i].idx
		}
	}
}

func mapinit() map[uint64]*wheelinfo {

	bmap := make(map[uint64]*wheelinfo, max)
	for i := uint64(0); i < max; i++ {
		bmap[i] = &wheelinfo{idx: i}
	}

	return bmap
}

func sliceinit() []*wheelinfo {

	bslice := make([]*wheelinfo, max)
	for i := uint64(0); i < max; i++ {
		bslice[i] = &wheelinfo{idx: i}
	}

	return bslice
}
