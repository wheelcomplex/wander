// +build linux

package main

import (
	"net"
	"os"
	"sync"
	"syscall"
	"unsafe"
)

const (
	// SPLICE_F_MOVE hints to the kernel to
	// move page references rather than memory.
	fSpliceMove = 0x01

	// NOTE: SPLICE_F_NONBLOCK only makes the pipe operations
	// non-blocking. A pipe operation could still block if the
	// underlying fd was set to blocking. Conversely, a call
	// to splice() without SPLICE_F_NONBLOCK can still return
	// EAGAIN if the non-pipe fd is non-blocking.
	fSpliceNonblock = 0x02

	// SPLICE_F_MORE hints that more data will be sent
	// (see: TCP_CORK).
	fSpliceMore = 0x04

	// In *almost* all Linux kernels, pipes are this size,
	// so we can use it as a size hint when filling a pipe.
	pipeBuf = 65535
)

func rawsplice(srcfd int, dstfd int, amt int, flags int) (int64, syscall.Errno) {
	r, _, e := syscall.RawSyscall6(syscall.SYS_SPLICE, uintptr(srcfd), 0, uintptr(dstfd), 0, uintptr(amt), uintptr(flags))
	return int64(r), e
}

var _zero uintptr

func rawEpollWait(epfd int, events []syscall.EpollEvent, msec int) (n int, err syscall.Errno) {
	var _p0 unsafe.Pointer
	if len(events) > 0 {
		_p0 = unsafe.Pointer(&events[0])
	} else {
		_p0 = unsafe.Pointer(&_zero)
	}
	r0, _, e1 := syscall.RawSyscall6(syscall.SYS_EPOLL_WAIT, uintptr(epfd), uintptr(_p0), uintptr(len(events)), uintptr(msec), 0, 0)
	n = int(r0)
	err = e1
	return
}

const (
	EPOLLIN    = syscall.EPOLLIN
	EPOLLOUT   = syscall.EPOLLOUT
	EPOLLERR   = syscall.EPOLLERR
	EPOLLHUP   = syscall.EPOLLHUP
	EPOLLET    = syscall.EPOLLET
	EPOLLRDHUP = syscall.EPOLLRDHUP

	EPOLL_CTL_ADD = syscall.EPOLL_CTL_ADD
	EPOLL_CTL_DEL = syscall.EPOLL_CTL_DEL
	EPOLL_CTL_MOD = syscall.EPOLL_CTL_MOD
)

type Proxy struct {
	fd    int
	mutex sync.Mutex
	conns map[int32]*proxyConn
}

func newProxy() *Proxy {
	fd, err := syscall.EpollCreate(10)
	if err != nil {
		return nil
	}
	p := &Proxy{
		fd:    fd,
		conns: make(map[int32]*proxyConn, 10000),
	}
	go p.eventLoop()
	return p
}

func (p *Proxy) addConn(conn *proxyConn) {
	p.mutex.Lock()
	p.conns[int32(conn.fd)] = conn
	p.mutex.Unlock()
}

func (p *Proxy) delConn(conn *proxyConn) {
	p.mutex.Lock()
	if _, ok := p.conns[int32(conn.fd)]; ok {
		conn.Close(p.fd)
		delete(p.conns, int32(conn.fd))
	}
	p.mutex.Unlock()
}

func (p *Proxy) getConn(fd int32) (conn *proxyConn) {
	p.mutex.Lock()
	conn, _ = p.conns[fd]
	p.mutex.Unlock()
	return
}

func (p *Proxy) eventLoop() {
	var events [64]syscall.EpollEvent
	for {
		//println("epoll wait begin")
		//t1 := time.Now()
		n, err := syscall.EpollWait(p.fd, events[:], -1)
		if err != nil {
			break
		}
		////println("epoll wait done:", time.Since(t1).String())
		for i := 0; i < n; i++ {
			//t1 := time.Now()
			conn := p.getConn(events[i].Fd)
			if conn == nil {
				continue
			}
			evs := events[i].Events

			if evs&(EPOLLERR|EPOLLHUP|EPOLLRDHUP) != 0 {
				//println("evs&(EPOLLERR|EPOLLHUP|EPOLLRDHUP) != 0")
				p.delConn(conn)
				continue
			}

			if evs&EPOLLIN != 0 {
				if !conn.MoveIn(p.fd) {
					//println("evs&EPOLLIN != 0")
					p.delConn(conn)
					continue
				}
				if conn.other.canWrite {
					if !conn.other.MoveOut(p.fd) {
						//println("evs&EPOLLOUT != 0")
						p.delConn(conn.other)
						continue
					}
				}
			}

			if evs&EPOLLOUT != 0 {
				if !conn.MoveOut(p.fd) {
					//println("evs&EPOLLOUT != 0")
					p.delConn(conn)
					continue
				}
				if conn.other.canRead {
					if !conn.other.MoveIn(p.fd) {
						//println("evs&EPOLLOUT != 0")
						p.delConn(conn.other)
						continue
					}
				}
			}
			////println("event:", time.Since(t1).String())
		}
		////println()
	}
}

func (p *Proxy) Transfer(conn1, conn2 net.Conn) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	pc1, err := p.newConn(conn1)
	if err != nil {
		return err
	}

	pc2, err := p.newConn(conn2)
	if err != nil {
		return err
	}

	pc1.other, pc2.other = pc2, pc1
	return nil
}

type proxyConn struct {
	fd       int
	file     *os.File
	rPipe    int
	wPipe    int
	inBuf    int64
	other    *proxyConn
	canRead  bool
	canWrite bool
}

func (p *Proxy) newConn(conn net.Conn) (*proxyConn, error) {
	file, err := conn.(*net.TCPConn).File()
	if err != nil {
		return nil, err
	}
	conn.Close()

	var pipeFD [2]int
	err = syscall.Pipe2(pipeFD[:], syscall.O_CLOEXEC|syscall.O_NONBLOCK)
	if err != nil {
		return nil, err
	}

	pc := &proxyConn{
		fd:    int(file.Fd()),
		file:  file,
		rPipe: pipeFD[0],
		wPipe: pipeFD[1],
	}

	if err = syscall.SetNonblock(pc.fd, true); err != nil {
		return nil, err
	}

	err = syscall.EpollCtl(p.fd, EPOLL_CTL_ADD, pc.fd, &syscall.EpollEvent{
		Fd:     int32(pc.fd),
		Events: EPOLLIN | EPOLLOUT | EPOLLRDHUP | (EPOLLET & 0xffffffff),
	})
	if err != nil {
		pc.Dispose()
		return nil, err
	}

	p.conns[int32(pc.fd)] = pc
	return pc, nil
}

func (p *proxyConn) Close(poll int) {
	syscall.EpollCtl(poll, EPOLL_CTL_DEL, p.fd, &syscall.EpollEvent{})
	p.Dispose()
}

func (p *proxyConn) Dispose() {
	syscall.Close(p.fd)
	syscall.Close(p.rPipe)
	syscall.Close(p.wPipe)
}

func (p *proxyConn) MoveIn(poll int) bool {
	//println("MoveIn", p.fd, p.inBuf)
	p.canRead = true
	for p.inBuf < pipeBuf {
		//println("MoveIn splice")
		n, eno := rawsplice(p.fd, p.wPipe, int(pipeBuf-p.inBuf), fSpliceMove|fSpliceNonblock)
		//n, err := syscall.Splice(p.fd, nil, p.wPipe, nil, int(pipeBuf-p.inBuf), fSpliceMove|fSpliceNonblock)
		if n == 0 {
			return false
		}
		if n < 0 {
			//eno := err.(syscall.Errno)
			if eno == syscall.EAGAIN || eno == syscall.EWOULDBLOCK {
				break
			}
			//println("r err:", eno)
			return false
		}
		p.inBuf += n
		p.canRead = false
	}
	//println("MoveIn done", p.fd, p.inBuf)
	return true
}

func (p *proxyConn) MoveOut(poll int) bool {
	//println("MoveOut", p.fd, p.other.inBuf)
	p.canWrite = true
	for p.other.inBuf > 0 {
		//println("MoveOut splice")
		n, eno := rawsplice(p.other.rPipe, p.fd, int(p.other.inBuf), fSpliceMove|fSpliceNonblock)
		//n, err := syscall.Splice(p.other.rPipe, nil, p.fd, nil, int(p.other.inBuf), fSpliceMove|fSpliceNonblock)
		if n == 0 {
			break
		}
		if n < 0 {
			//eno := err.(syscall.Errno)
			if eno == syscall.EAGAIN || eno == syscall.EWOULDBLOCK {
				break
			}
			//println("w err:", eno)
			return false
		}
		p.other.inBuf -= n
		p.canWrite = false
	}
	//println("MoveOut done", p.fd, p.other.inBuf)
	return true
}
