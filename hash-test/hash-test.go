package main

import (
	"bytes"
	"flag"
	"fmt"
	"math"
	"math/big"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"github.com/wheelcomplex/preinit/bigcounter"
	"github.com/wheelcomplex/preinit/cmtp"
	"github.com/wheelcomplex/preinit/misc"

	"tpf"
)

// newByteChunk new [][]byte for sync.Pool.New
func newByteChunk(chunksize, hashsize int) func() interface{} {
	return func() interface{} {
		c := make([][]byte, chunksize)
		for i := 0; i < chunksize; i++ {
			c[i] = make([]byte, hashsize)
		}
		return c
	}
}

//
type hashCarrier struct {
	ptr int
	buf []uint32
}

// newHashChunk new []uint32 for sync.Pool.New
func newHashChunk(chunksize int) func() interface{} {
	return func() interface{} {
		return &hashCarrier{
			buf: make([]uint32, chunksize),
		}
	}
}

//
type hashTester struct {
	size       int                   //
	chunksize  int                   //
	groupsize  int                   //
	generator  bigcounter.BigCounter // standalone in worker
	checksum   cmtp.Checksum         // standalone in worker
	counterCh  chan uint64           // result out, []uint32
	bytesumCh  []chan *hashCarrier   // cross worker checksum chunk
	hashPool   []*sync.Pool          //
	count      bigcounter.BigCounter // counter for qps compute
	maxcnt     *big.Int              //
	bigSecond  *big.Int              //
	results    []uint16              // all hash info
	distr      []uint64              // index by math.MaxUint32 % groupsize
	collided   []uint64              // index by math.MaxUint32 % groupsize
	startts    time.Time             //
	endts      time.Time             //
	esp        *big.Int              //
	qps        *big.Int              //
	limit      *time.Timer           //
	closed     chan struct{}         //
	closing    chan struct{}         //
	maxworker  int                   //
	maxcounter int                   //
	lockThread bool                  //
	finished   bool                  //
	killworker bool                  //
	m          sync.Mutex            //
	rm         sync.Mutex            // result locker
}

//
func NewhashTester(size int,
	generator bigcounter.BigCounter,
	checksum cmtp.Checksum,
	groupsize int,
	limit int64,
	lockThread bool) *hashTester {

	// do not lock os thread when only one cpu used
	if runtime.GOMAXPROCS(-1) == 1 {
		lockThread = false
	}

	maxcounter := 1
	maxworker := 1
	if runtime.GOMAXPROCS(-1) > 12 {
		maxcounter := runtime.GOMAXPROCS(-1) / 12
		if maxcounter < 1 {
			maxcounter = 1
		}
		// reserve one cpu for common task
		maxworker = runtime.GOMAXPROCS(-1) - maxcounter - 1
	} else {
		maxworker = runtime.GOMAXPROCS(-1) - maxcounter
	}
	if maxworker < 1 {
		maxworker = 1
	}
	misc.Tpf("initialing %d workers, %d counters ...\n", maxworker, maxcounter)
	tpf.Tpf("initialing %d workers, %d counters ...\n", maxworker, maxcounter)
	os.Exit(1)

	if size < 1 {
		size = 1
	}

	// larger chunksize use more memory and save more cpu
	chunksize := 1024
	countersize := 2048
	// 3228

	if groupsize <= 0 {
		groupsize = 4096
	}

	if limit <= 0 {
		limit = math.MaxUint32
	}

	maxcnt, _ := big.NewInt(0).SetString(generator.Max(), 10)
	//fmt.Printf("generator.Size() %d, generator.Max() = %s || %x => %s || %x\n", generator.Size(), generator.Max(), generator.Bytes(), maxcnt.String(), maxcnt.Bytes())

	ht := &hashTester{
		size:       size,
		chunksize:  chunksize,
		groupsize:  groupsize,
		generator:  generator,
		checksum:   checksum,
		counterCh:  make(chan uint64, countersize),
		bytesumCh:  make([]chan *hashCarrier, maxcounter),
		hashPool:   make([]*sync.Pool, maxcounter),
		count:      generator.New(),
		maxcnt:     maxcnt,
		bigSecond:  big.NewInt(int64(time.Second)),
		results:    make([]uint16, math.MaxUint32+1),
		distr:      make([]uint64, groupsize),
		collided:   make([]uint64, groupsize),
		startts:    time.Now(),
		endts:      time.Now(),
		qps:        big.NewInt(0),
		closed:     make(chan struct{}, 128),
		closing:    make(chan struct{}, 128),
		limit:      time.NewTimer(time.Duration(limit) * time.Second),
		maxworker:  maxworker,
		maxcounter: maxcounter,
		lockThread: lockThread,
	}
	//
	for i := 0; i < ht.maxcounter; i++ {
		ht.bytesumCh[i] = make(chan *hashCarrier, ht.chunksize*ht.maxworker)
		ht.hashPool[i] = &sync.Pool{
			New: newHashChunk(chunksize),
		}
	}

	// initial stat
	ht.Stat()
	go ht.closer()
	go ht.counter()
	go ht.run()
	return ht
}

//
func (ht *hashTester) run() {
	var wg sync.WaitGroup
	//misc.Tpf(fmt.Sprintln("lauch", ht.maxworker, "worker"))

	// initial generator step
	genstep := big.NewInt(0)
	genstep = genstep.Div(ht.maxcnt, big.NewInt(int64(ht.maxworker)))
	genptr := big.NewInt(0)
	ht.startts = time.Now()
	for i := 0; i < ht.maxworker; i++ {
		// initial worker
		generator := ht.generator.New()
		generator.SetInit(genptr.Bytes())
		endptr := generator.New()
		endptr.FromBigInt(big.NewInt(1).Mul(genstep, big.NewInt(int64(i+1))))
		endptr.Mimus()
		if i == ht.maxworker-1 {
			generator.SetMax(ht.maxcnt.Bytes())
		} else {
			generator.SetMax(endptr.Bytes())
		}
		//println("generator#", i, "start from", generator.String(), "end at", generator.Max())
		genptr = genptr.Add(genptr, genstep)

		wg.Add(1)
		go ht.runWorker(uint32(i), generator, &wg)
		//time.Sleep(2)
	}
	//
	genptr = nil
	genstep = nil

	misc.Tpf(fmt.Sprintln(ht.maxworker, "worker running ..."))
	ht.Stat()
	wg.Wait()
	ht.Stat()
	select {
	case <-ht.closing:
	default:
		close(ht.closing)
	}
	// all done, close
	for i, _ := range ht.bytesumCh {
		close(ht.bytesumCh[i])
	}
	close(ht.counterCh)
	misc.Tpf(fmt.Sprintln(ht.maxworker, "worker exited"))
}

//
func (ht *hashTester) runWorker(index uint32, generator bigcounter.BigCounter, wg *sync.WaitGroup) {
	defer func() {
		wg.Done()
	}()
	if ht.lockThread {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
	}

	var bchunk [][]byte

	// TODO: compare select closing check and bool closing check

	bytePool := sync.Pool{
		New: newByteChunk(ht.chunksize, ht.size),
	}

	checksum := ht.checksum.New(0)
	counteridx := uint32(0)
	maxcounter := uint32(ht.maxcounter)
	chunkcount := uint64(0)

	hchunk := make([]*hashCarrier, ht.maxcounter)

	//misc.Tpf(fmt.Sprintln("worker", worker.index, "running"))
	for ht.killworker == false {
		bchunk = bytePool.Get().([][]byte)
		for i := 0; i < ht.maxcounter; i++ {
			hchunk[i] = ht.hashPool[i].Get().(*hashCarrier)
			hchunk[i].ptr = -1
		}
		chunkcount = 0
		for idx, _ := range bchunk {
			// generate hash buffer
			// FillBytes FillExpBytes
			generator.FillExpBytes(bchunk[idx])

			// hash and save result
			bytesum := checksum.Checksum32(bchunk[idx])

			counteridx = bytesum % maxcounter
			hchunk[counteridx].ptr++
			hchunk[counteridx].buf[hchunk[counteridx].ptr] = bytesum

			// generator ++
			if err := generator.Plus(); err != nil {
				// last chunk
				bytePool.Put(bchunk)
				for i, _ := range hchunk {
					ht.bytesumCh[i] <- hchunk[i]
					chunkcount += uint64(hchunk[i].ptr) + 1
				}
				ht.counterCh <- chunkcount
				return
			}
		}

		bytePool.Put(bchunk)

		for i, _ := range hchunk {
			ht.bytesumCh[i] <- hchunk[i]
			chunkcount += uint64(hchunk[i].ptr) + 1
		}
		ht.counterCh <- chunkcount
	}
	//misc.Tpf(fmt.Sprintln("worker", worker.index, "exited"))
}

func (ht *hashTester) counter() {
	defer func() {
		ht.Close()
	}()
	var wg sync.WaitGroup
	wg.Add(1)
	go ht.totalcounter(&wg)

	for i := 0; i < ht.maxcounter; i++ {
		wg.Add(1)
		go ht.sumcounter(uint32(i), &wg)
	}
	misc.Tpf(fmt.Sprintln(ht.maxcounter, "counter running ..."))
	wg.Wait()
	// worker exited
	ht.Stat()
	ht.summary()
	close(ht.closed)
}

func (ht *hashTester) sumcounter(index uint32, wg *sync.WaitGroup) {
	defer func() {
		wg.Done()
	}()
	if ht.lockThread && ht.maxcounter > 1 {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
	}
	// []*hashCarrier
	for sumbock := range ht.bytesumCh[index] {
		for i := 0; i <= sumbock.ptr; i++ {
			ht.results[sumbock.buf[i]]++
		}
		ht.hashPool[index].Put(sumbock)
	}

}

func (ht *hashTester) totalcounter(wg *sync.WaitGroup) {
	defer func() {
		wg.Done()
	}()
	for count := range ht.counterCh {
		ht.rm.Lock()
		ht.count.AddUint64(count)
		ht.rm.Unlock()
	}
}

func (ht *hashTester) summary() {
	misc.Tpf(fmt.Sprintln("result computing"))
	for idx, val := range ht.results {
		ridx := idx % ht.groupsize
		//if val != 0 {
		//	println("idx", idx, "count", val, "ridx", ridx)
		//}
		//results     []uint16        // all hash info
		//distr       []uint64        // index by math.MaxUint32 % groupsize
		//collided    []uint64        // index by math.MaxUint32 % groupsize
		ht.distr[ridx]++
		if val > 0 {
			ht.collided[ridx]++
		}
	}
	misc.Tpf(fmt.Sprintln("result computed"))
}

// show qps
func (ht *hashTester) Stat() (countstr, qps string, esp time.Duration) {
	select {
	case <-ht.closing:
		// already closing, do not update
	case <-ht.closed:
		// already closed, do not update
	default:
		ht.endts = time.Now()
	}
	esp = ht.endts.Sub(ht.startts)
	ht.rm.Lock()
	count := ht.count.ToBigInt()
	ht.rm.Unlock()
	//fmt.Printf("Stat(), esp %v, count %s, %s\n", esp, ht.count.String(), count.String())
	ht.esp = big.NewInt(int64(esp.Nanoseconds()))
	ht.qps = ht.qps.Mul(count, ht.bigSecond)
	ht.qps = ht.qps.Div(ht.qps, ht.esp)
	countstr, qps = count.String(), ht.qps.String()
	return
}

// Size
func (ht *hashTester) Size() int {
	return int(ht.size)
}

//
func (ht *hashTester) closer() {
	defer func() {
		ht.limit.Stop()
		// stop all workers
		ht.killworker = true
	}()
	select {
	case <-ht.limit.C:
		misc.Tpf("%s\n", "stop for reach time limit")
		return
	case <-ht.closing:
		misc.Tpf("%s\n", "stop for closing")
		return
	}
}

// Close free memory
func (ht *hashTester) Close() {
	ht.m.Lock()
	defer ht.m.Unlock()
	if ht.finished == true {
		return
	}
	select {
	case <-ht.closing:
	default:
		close(ht.closing)
	}
	<-ht.closed
	//misc.Tpffmt.Sprintln("closing"))
	ht.results = nil
	//ht.hashPool = nil
	//ht.collided = nil
	//ht.distr = nil
	//misc.Tpffmt.Sprintln("GC"))
	runtime.GC()
	//misc.Tpffmt.Sprintln("FreeMemory"))
	debug.FreeOSMemory()
	//misc.Tpffmt.Sprintln("closed"))
	ht.finished = true
}

// Result return distr/collided map
// caller will blocked until finished
func (ht *hashTester) Result() ([]uint64, []uint64) {
	<-ht.closed
	return ht.distr, ht.collided
}

//
func (ht *hashTester) Wait() <-chan struct{} {
	return ht.closed
}

//
//
// github.com/wheelcomplex/svgo
//

//
// http://faso.me/notes/20140411/svg-tutorial/
//
//
// http://apike.ca/prog_svg_jsanim.html javascript in svg
//
// type="text/ecmascript" for local svg file
// type="text/javascript" for web browser
//
type SvgDrawServer struct {
	size     int
	distr    map[string][]uint64
	collided map[string][]uint64
	svgbuf   []byte
	buf      []byte
	header   []byte
	tail     []byte
	m        sync.Mutex
	wait     chan struct{}
	count    uint64
}

//
func newSvgDrawServer(size int) *SvgDrawServer {
	if size <= 0 {
		size = 1
	}
	var header []byte = []byte(`<?xml version="1.0" encoding="UTF-8" standalone="no"?>

<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd">

<svg width="100%" height="660" xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink">

<!-- from https://www.cyberz.org/projects/SVGPan/SVGPan.js -->
<script type="text/ecmascript"><![CDATA[
/** 
 *  SVGPan library 1.2.1
 * ======================
 *
 * Given an unique existing element with id "viewport" (or when missing, the first g 
 * element), including the the library into any SVG adds the following capabilities:
 *
 *  - Mouse panning
 *  - Mouse zooming (using the wheel)
 *  - Object dragging
 *
 * You can configure the behaviour of the pan/zoom/drag with the variables
 * listed in the CONFIGURATION section of this file.
 *
 * Known issues:
 *
 *  - Zooming (while panning) on Safari has still some issues
 *
 * Releases:
 *
 * 1.2.1, Mon Jul  4 00:33:18 CEST 2011, Andrea Leofreddi
 *	- Fixed a regression with mouse wheel (now working on Firefox 5)
 *	- Working with viewBox attribute (#4)
 *	- Added "use strict;" and fixed resulting warnings (#5)
 *	- Added configuration variables, dragging is disabled by default (#3)
 *
 * 1.2, Sat Mar 20 08:42:50 GMT 2010, Zeng Xiaohui
 *	Fixed a bug with browser mouse handler interaction
 *
 * 1.1, Wed Feb  3 17:39:33 GMT 2010, Zeng Xiaohui
 *	Updated the zoom code to support the mouse wheel on Safari/Chrome
 *
 * 1.0, Andrea Leofreddi
 *	First release
 *
 * This code is licensed under the following BSD license:
 *
 * Copyright 2009-2010 Andrea Leofreddi <a.leofreddi@itcharm.com>. All rights reserved.
 * 
 * Redistribution and use in source and binary forms, with or without modification, are
 * permitted provided that the following conditions are met:
 * 
 *    1. Redistributions of source code must retain the above copyright notice, this list of
 *       conditions and the following disclaimer.
 * 
 *    2. Redistributions in binary form must reproduce the above copyright notice, this list
 *       of conditions and the following disclaimer in the documentation and/or other materials
 *       provided with the distribution.
 * 
 * THIS SOFTWARE IS PROVIDED BY Andrea Leofreddi ''AS IS'' AND ANY EXPRESS OR IMPLIED
 * WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND
 * FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL Andrea Leofreddi OR
 * CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
 * CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
 * SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON
 * ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING
 * NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF
 * ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
 * 
 * The views and conclusions contained in the software and documentation are those of the
 * authors and should not be interpreted as representing official policies, either expressed
 * or implied, of Andrea Leofreddi.
 */

"use strict";

/// CONFIGURATION 
/// ====>

var enablePan = 1; // 1 or 0: enable or disable panning (default enabled)
var enableZoom = 1; // 1 or 0: enable or disable zooming (default enabled)
var enableDrag = 0; // 1 or 0: enable or disable dragging (default disabled)

/// <====
/// END OF CONFIGURATION 

var root = document.documentElement;

var state = 'none', svgRoot, stateTarget, stateOrigin, stateTf;

setupHandlers(root);

/**
 * Register handlers
 */
function setupHandlers(root){
	setAttributes(root, {
		"onmouseup" : "handleMouseUp(evt)",
		"onmousedown" : "handleMouseDown(evt)",
		"onmousemove" : "handleMouseMove(evt)",
		//"onmouseout" : "handleMouseUp(evt)", // Decomment this to stop the pan functionality when dragging out of the SVG element
	});

	if(navigator.userAgent.toLowerCase().indexOf('webkit') >= 0)
		window.addEventListener('mousewheel', handleMouseWheel, false); // Chrome/Safari
	else
		window.addEventListener('DOMMouseScroll', handleMouseWheel, false); // Others
}

/**
 * Retrieves the root element for SVG manipulation. The element is then cached into the svgRoot global variable.
 */
function getRoot(root) {
	if(typeof(svgRoot) == "undefined") {
		var g = null;

		g = root.getElementById("viewport");

		if(g == null)
			g = root.getElementsByTagName('g')[0];

		if(g == null)
			alert('Unable to obtain SVG root element');

		setCTM(g, g.getCTM());

		g.removeAttribute("viewBox");

		svgRoot = g;
	}

	return svgRoot;
}

/**
 * Instance an SVGPoint object with given event coordinates.
 */
function getEventPoint(evt) {
	var p = root.createSVGPoint();

	p.x = evt.clientX;
	p.y = evt.clientY;

	return p;
}

/**
 * Sets the current transform matrix of an element.
 */
function setCTM(element, matrix) {
	var s = "matrix(" + matrix.a + "," + matrix.b + "," + matrix.c + "," + matrix.d + "," + matrix.e + "," + matrix.f + ")";

	element.setAttribute("transform", s);
}

/**
 * Dumps a matrix to a string (useful for debug).
 */
function dumpMatrix(matrix) {
	var s = "[ " + matrix.a + ", " + matrix.c + ", " + matrix.e + "\n  " + matrix.b + ", " + matrix.d + ", " + matrix.f + "\n  0, 0, 1 ]";

	return s;
}

/**
 * Sets attributes of an element.
 */
function setAttributes(element, attributes){
	for (var i in attributes)
		element.setAttributeNS(null, i, attributes[i]);
}

/**
 * Handle mouse wheel event.
 */
function handleMouseWheel(evt) {
	if(!enableZoom)
		return;

	if(evt.preventDefault)
		evt.preventDefault();

	evt.returnValue = false;

	var svgDoc = evt.target.ownerDocument;

	var delta;

	if(evt.wheelDelta)
		delta = evt.wheelDelta / 3600; // Chrome/Safari
	else
		delta = evt.detail / -90; // Mozilla

	var z = 1 + delta; // Zoom factor: 0.9/1.1

	var g = getRoot(svgDoc);
	
	var p = getEventPoint(evt);

	p = p.matrixTransform(g.getCTM().inverse());

	// Compute new scale matrix in current mouse position
	var k = root.createSVGMatrix().translate(p.x, p.y).scale(z).translate(-p.x, -p.y);

        setCTM(g, g.getCTM().multiply(k));

	if(typeof(stateTf) == "undefined")
		stateTf = g.getCTM().inverse();

	stateTf = stateTf.multiply(k.inverse());
}

/**
 * Handle mouse move event.
 */
function handleMouseMove(evt) {
	if(evt.preventDefault)
		evt.preventDefault();

	evt.returnValue = false;

	var svgDoc = evt.target.ownerDocument;

	var g = getRoot(svgDoc);

	if(state == 'pan' && enablePan) {
		// Pan mode
		var p = getEventPoint(evt).matrixTransform(stateTf);

		setCTM(g, stateTf.inverse().translate(p.x - stateOrigin.x, p.y - stateOrigin.y));
	} else if(state == 'drag' && enableDrag) {
		// Drag mode
		var p = getEventPoint(evt).matrixTransform(g.getCTM().inverse());

		setCTM(stateTarget, root.createSVGMatrix().translate(p.x - stateOrigin.x, p.y - stateOrigin.y).multiply(g.getCTM().inverse()).multiply(stateTarget.getCTM()));

		stateOrigin = p;
	}
}

/**
 * Handle click event.
 */
function handleMouseDown(evt) {
	if(evt.preventDefault)
		evt.preventDefault();

	evt.returnValue = false;

	var svgDoc = evt.target.ownerDocument;

	var g = getRoot(svgDoc);

	if(
		evt.target.tagName == "svg" 
		|| !enableDrag // Pan anyway when drag is disabled and the user clicked on an element 
	) {
		// Pan mode
		state = 'pan';

		stateTf = g.getCTM().inverse();

		stateOrigin = getEventPoint(evt).matrixTransform(stateTf);
	} else {
		// Drag mode
		state = 'drag';

		stateTarget = evt.target;

		stateTf = g.getCTM().inverse();

		stateOrigin = getEventPoint(evt).matrixTransform(stateTf);
	}
}

/**
 * Handle mouse button release event.
 */
function handleMouseUp(evt) {
	if(evt.preventDefault)
		evt.preventDefault();

	evt.returnValue = false;

	var svgDoc = evt.target.ownerDocument;

	if(state == 'pan' || state == 'drag') {
		// Quit pan mode
		state = '';
	}
}
]]></script>

<g id="viewport" transform="scale(1, 1) translate(0, 0)">

<!-- X line 80 - 1280 = 10180
<line x1="40" y1="500" x2="10260" y2="500" style="stroke:#2E9AFE; stroke-width:1" />
-->

<text x="28" y="506" fill="#FF3D82" font-size="16">X</text>

<!-- Y line 20 - 500 = 480 
<line x1="80" y1="20" x2="80" y2="640" style="stroke:#2E9AFE; stroke-width:1" />
-->

<text x="74" y="18" fill="#FF3D82" font-size="16">Y</text>

<text x="64" y="520" fill="#FF3D82" font-size="24">0</text>
`)

	var tail []byte = []byte(`
</g>
</svg>`)

	return &SvgDrawServer{
		size:     size,
		header:   header,
		tail:     tail,
		distr:    make(map[string][]uint64),
		collided: make(map[string][]uint64),
		svgbuf:   make([]byte, 2048),
		buf:      make([]byte, 2048),
		wait:     make(chan struct{}),
	}
}

//
func (sh *SvgDrawServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	sh.m.Lock()
	defer sh.m.Unlock()
	sh.count++

	w.Header().Set("Content-Type", "image/svg+xml;charset=UTF-8")
	// Refresh: 0;url=my_view_page.php
	//w.Header().Set("Refresh", "15;"+req.URL.RequestURI())
	//
	// create desc
	//
	sh.svgbuf = sh.svgbuf[:0]

	// sh.header
	sh.svgbuf = append(sh.svgbuf, sh.header...)

	color := uint8(0)
	if len(sh.distr) > 0 {
		xname := 180
		for idx, _ := range sh.distr {
			color++
			if color == math.MaxUint8 {
				color = 1
			}
			sh.svgbuf = append(sh.svgbuf, []byte(`<text x="`+fmt.Sprintf("%d", xname)+`" y="560" fill="`+fmt.Sprintf("#%02x9AFE", color)+`" font-size="24">`+idx+`</text>`+"\n")...)
			xname += (len(idx)*2 + 20)
		}
	} else {
		// desc
		sh.svgbuf = append(sh.svgbuf, []byte(`<text x="180" y="560" fill="#DF3D82" font-size="24"> testing, please wait ...</text>`+"\n")...)
	}
	// time stamp
	sh.svgbuf = append(sh.svgbuf, []byte(fmt.Sprintf("%s#%d, %s%s", `<text x="180" y="600" fill="#DF3D82" font-size="24">`, sh.count, time.Now().String(), `</text>`+"\n"))...)

	//
	// create svg
	//

	/*
	   M：move to ，移动至
	   L：line to ，直线至
	   V：vertical line to ，垂直方向直线至
	   H：horizontal line to ，水平方向直线至
	   C：curve to ，曲线至
	   S：smooth curve to ，平滑曲线至
	   Q：quadratic Bézier curve，二维贝塞尔曲线
	   T：smooth quadratic Bézier curve，平滑二维贝塞尔曲线
	   A：elliptical arc，椭圆弧

	   大写字母表示定位方式使用绝对位置，小写则使用相对定位
	*/

	/*
		<!-- X line 80 - 1280 = 1180-->
		<line x1="40" y1="500" x2="1280" y2="500" style="stroke:#2E9AFE; stroke-width:1" />

		<!-- Y line 20 - 500 = 480 -->
		<line x1="80" y1="20" x2="80" y2="640" style="stroke:#2E9AFE; stroke-width:1" />
	*/

	xstart := 80

	ystart := 500
	yend := 20

	yavg := uint64(((ystart - yend) / 2) + yend)

	sh.svgbuf = append(sh.svgbuf, []byte(fmt.Sprintf(`<text x="%d" y="%d" fill="#DF3D82" font-size="24">%d</text>`, xstart-40, yavg+8, yavg)+"\n")...)

	// draw x/y line
	xstep := 1
	maxlen := 1280
	if sh.size <= maxlen {
		xstep = maxlen / sh.size
	}
	xnextlen := sh.size / maxlen
	// x line
	sh.svgbuf = append(sh.svgbuf, []byte("\n"+`<line x1="40" y1="500" x2="`+"\n")...)
	sh.svgbuf = append(sh.svgbuf, []byte(fmt.Sprintf("%d", maxlen+xstart))...)
	sh.svgbuf = append(sh.svgbuf, []byte(`" y2="500" style="stroke:#2E9AFE; stroke-width:1" />`+"\n")...)
	// fixed y line
	sh.svgbuf = append(sh.svgbuf, []byte("\n"+`<line x1="80" y1="20" x2="80" y2="640" style="stroke:#2E9AFE; stroke-width:1" />`+"\n")...)

	// fixed x line
	//sh.svgbuf = append(sh.svgbuf, []byte("\n"+`<line x1="40" y1="500" x2="1280" y2="500" style="stroke:#2E9AFE; stroke-width:1" />`)...)

	if len(sh.distr) > 0 {
		// draw path
		color = uint8(0)
		for name, _ := range sh.distr {
			color++
			if color == math.MaxUint8 {
				color = 1
			}
			// start of path
			sh.svgbuf = append(sh.svgbuf, []byte("\n<path d=\"M")...)

			dist := uint64(0)
			avgdist := uint64(0)
			for i := 0; i < len(sh.distr[name]); i++ {
				dist += sh.distr[name][i]
				//if maxdist < sh.distr[name][i] {
				//	maxdist = sh.distr[name][i]
				//}
				//if mindist > sh.distr[name][i] {
				//	mindist = sh.distr[name][i]
				//}
			}
			avgdist = dist / uint64(len(sh.distr[name]))
			xptr := xstart
			sh.buf = sh.buf[:0]
			xpos := 0
			xcnt := 0
			ypre := uint64(0)
			for i := 0; i < len(sh.distr[name]); i++ {
				dist = (sh.distr[name][i] - avgdist) + yavg
				if i-xpos >= xnextlen {
					xpos = i
					xptr = xcnt*xstep + xstart
					xcnt++
				}
				if ypre != dist || i == len(sh.distr[name])-1 {
					ypre = dist
					println("#", sh.count, "xnextlen", xnextlen, "i", i, "xcnt", xcnt, "xstep", xstep, "xptr", xptr, "sh.distr[name][i]", sh.distr[name][i], "avgdist", avgdist, "raw", (sh.distr[name][i] - avgdist), "dist", dist)
				}
				if i == 0 {
					sh.buf = append(sh.buf, []byte(fmt.Sprintf("%d %d S", xptr, dist))...)
				} else {
					sh.buf = append(sh.buf, []byte(fmt.Sprintf("%d %d ", xptr, dist))...)
				}
			}
			sh.buf = bytes.Trim(sh.buf, " ")
			sh.svgbuf = append(sh.svgbuf, sh.buf...)
			// end of path
			sh.svgbuf = append(sh.svgbuf, []byte(`" style="fill: none; stroke: `+fmt.Sprintf("#%02x9AFE", color)+`; stroke-width: 2;" />`+"\n")...)
		}

		//
		// sh.svgbuf = append(sh.svgbuf, []byte(fmt.Sprintf("%s", "140 30 180 90 20 160"))...)
	} else {
		// nothing to draw
	}

	// sh.tail
	sh.svgbuf = append(sh.svgbuf, sh.tail...)

	outputlen := len(sh.svgbuf)
	//
	w.Header().Set("Content-Length", fmt.Sprintf("%d", outputlen))
	//
	w.Write(sh.svgbuf[:outputlen])
	println("")
}

//
func (sh *SvgDrawServer) Close() {
	defer func() {
		recover()
	}()
	close(sh.wait)
}

//
func (sh *SvgDrawServer) Wait() <-chan struct{} {
	return sh.wait
}

//
func (sh *SvgDrawServer) String() string {
	sh.m.Lock()
	defer sh.m.Unlock()
	sh.buf = sh.buf[:0]
	for name, _ := range sh.distr {
		sh.buf = append(sh.buf, []byte(fmt.Sprintf("--- distr %s ---\n", name))...)
		for idx, _ := range sh.distr[name] {
			sh.buf = append(sh.buf, []byte(fmt.Sprintf("%d, %d\n", idx, sh.distr[name][idx]))...)
		}
		sh.buf = append(sh.buf, []byte(fmt.Sprintf("--- collided %s ---\n", name))...)
		for idx, _ := range sh.collided[name] {
			if sh.collided[name][idx] > 0 {
				sh.buf = append(sh.buf, []byte(fmt.Sprintf("%d, %d\n", idx, sh.collided[name][idx]))...)
			}
		}
	}
	return string(sh.buf)
}

//
func (sh *SvgDrawServer) Fill(name string, filldistr, fillcollided []uint64) {
	sh.m.Lock()
	defer sh.m.Unlock()
	if _, ok := sh.distr[name]; ok == false {
		sh.distr[name] = make([]uint64, sh.size)
	}
	for i := 0; i < sh.size && i < len(filldistr); i++ {
		sh.distr[name][i] = filldistr[i]
	}
	// len(filldistr) < sh.size
	for i := len(filldistr); i < sh.size; i++ {
		sh.distr[name][i] = 0
	}
	//
	if _, ok := sh.collided[name]; ok == false {
		sh.collided[name] = make([]uint64, sh.size)
	}
	for i := 0; i < sh.size && i < len(fillcollided); i++ {
		sh.collided[name][i] = fillcollided[i]
	}
	// len(fillcollided) < sh.size
	for i := len(fillcollided); i < sh.size; i++ {
		sh.collided[name][i] = 0
	}
}

func main() {
	svgport := flag.String("svgport", ":9980", "svg http port")
	profileport := flag.Int("port", 6060, "profile http port")
	runlimit := flag.Int("time", 60, "run time(seconds) limit for each hash")
	size := flag.Int("size", 256, "block size")
	countsize := flag.Int("counter", 8, "counter size")
	groupsize := flag.Int("groupsize", 2048, "group size")
	cpus := flag.Int("cpu", 0, "cpus")
	lock := flag.Bool("lock", true, "lock os thread")
	stat := flag.Bool("stat", true, "show interval stat")
	flag.Parse()
	fmt.Printf(" go tool pprof http://localhost:%d/debug/pprof/profile\n", *profileport)
	fmt.Printf(" svg output http://localhost%s/\n", *svgport)
	go func() {
		fmt.Println(http.ListenAndServe(fmt.Sprintf("localhost:%d", *profileport), nil))
	}()

	if *cpus <= 0 {
		*cpus = runtime.NumCPU()
	}
	runtime.GOMAXPROCS(*cpus)
	if *size < 1 {
		*size = 1
	}

	if *groupsize < 16 {
		*groupsize = 16
	}

	if *runlimit < 1 {
		*runlimit = 1
	}

	svgl, svgerr := net.Listen("tcp", *svgport)
	if svgerr != nil {
		misc.Tpf("listen at %s failed: %s\n", svgerr.Error())
		os.Exit(1)
	}

	//
	allhasher := map[string]cmtp.Checksum{
		//"Murmur3": cmtp.NewMurmur3(0),
		"xxhash": cmtp.NewXxhash(0),
		//"noop":    cmtp.NewNoopChecksum(0),
		//"xxhash1": cmtp.NewXxhash(0),
		//"xxhash2": cmtp.NewXxhash(0),
		//"xxhash3": cmtp.NewXxhash(0),
	}
	//
	misc.Tpf("testing")
	for idx, _ := range allhasher {
		fmt.Printf(" %s", idx)
	}
	fmt.Printf(" ...\n")

	//
	// svgServer implated
	//
	// ServeHTTP(http.ResponseWriter, *http.Request)
	//

	svgServer := newSvgDrawServer(*groupsize)

	go func() {
		svgerr = http.Serve(svgl, svgServer)
		if svgerr != nil {
			misc.Tpf("Serve at %s failed: %s\n", svgerr.Error())
			os.Exit(1)
		}
		//
	}()

	for idx, onehash := range allhasher {
		misc.Tpf("timelimit %d seconds, counter size %d, size %d, groupsize %d, cpus %d, stat %v, lock os thread %v, start %s test\n", *runlimit, *countsize, *size, *groupsize, runtime.GOMAXPROCS(-1), *stat, *lock, idx)
		ht := NewhashTester(*size, bigcounter.NewAnyBaseCounter(*countsize), onehash, *groupsize, int64(*runlimit), *lock)
		waitCh := ht.Wait()
		if *stat {
			go func() {
				tk := time.NewTicker(5e9)
				defer tk.Stop()
				var preesp time.Duration
				for {
					select {
					case <-waitCh:
						return
					case <-tk.C:
						count, qps, esp := ht.Stat()
						if preesp != esp {
							misc.Tpf("Int %s, size %d, count %s, esp %v, qps %s\n", idx, *size, count, esp, qps)
							preesp = esp
						}
					}
				}
			}()
		}
		<-waitCh
		distr, collided := ht.Result()
		svgServer.Fill(idx, distr, collided)
		count, qps, esp := ht.Stat()
		misc.Tpf("End %s, size %d, count %s, esp %v, qps %s\n", idx, *size, count, esp, qps)
		misc.Tpf("%s test done\n", idx)
	}

	//fmt.Print(svgServer.String())

	misc.Tpf("all %d done\n", len(allhasher))

	<-svgServer.Wait()
}
