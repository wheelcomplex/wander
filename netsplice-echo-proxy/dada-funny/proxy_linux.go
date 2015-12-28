// +build linux

package lab04

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

type dummpMutex struct{}

func (dm dummpMutex) Lock()   {}
func (dm dummpMutex) Unlock() {}

type netPD struct {
	wcond     sync.Cond
	rcond     sync.Cond
	rWaitFlag int32
	wWaitFlag int32
	err       error
}

func (pd *netPD) writeLock() error {
	if atomic.CompareAndSwapInt32(&pd.wWaitFlag, 0, 1) {
		return nil
	}
	// TODO: error detail
	return EpollError
}

func (pd *netPD) writeUnlock() error {
	if atomic.CompareAndSwapInt32(&pd.wWaitFlag, 1, 0) {
		return nil
	}
	// TODO: error detail
	return EpollError
}

func (pd *netPD) readLock() error {
	if atomic.CompareAndSwapInt32(&pd.rWaitFlag, 0, 1) {
		return nil
	}
	// TODO: error detail
	return EpollError
}

func (pd *netPD) readUnlock() error {
	if atomic.CompareAndSwapInt32(&pd.rWaitFlag, 1, 0) {
		return nil
	}
	// TODO: error detail
	return EpollError
}

func (pd *netPD) WaitWrite() error {
	if atomic.CompareAndSwapInt32(&pd.wWaitFlag, 1, 2) {
		pd.wcond.Wait()
		if atomic.CompareAndSwapInt32(&pd.wWaitFlag, 2, 1) {
			return pd.err
		}
	}
	return EpollError
}

func (pd *netPD) WaitRead() error {
	if atomic.CompareAndSwapInt32(&pd.rWaitFlag, 1, 2) {
		pd.rcond.Wait()
		if atomic.CompareAndSwapInt32(&pd.rWaitFlag, 2, 1) {
			return pd.err
		}
	}
	return EpollError
}

func (pd *netPD) Close() {
	pd.err = EpollError

	for atomic.LoadInt32(&pd.wWaitFlag) != 3 {
		if atomic.CompareAndSwapInt32(&pd.wWaitFlag, 1, 3) {
			break
		}
		if atomic.CompareAndSwapInt32(&pd.wWaitFlag, 0, 3) {
			break
		}
		pd.wcond.Signal()
	}

	for atomic.LoadInt32(&pd.rWaitFlag) != 3 {
		if atomic.CompareAndSwapInt32(&pd.rWaitFlag, 1, 3) {
			break
		}
		if atomic.CompareAndSwapInt32(&pd.rWaitFlag, 0, 3) {
			break
		}
		pd.rcond.Signal()
	}
}

type netFD struct {
	sysfd int
	pd    netPD
}

func (fd *netFD) writeLock() error {
	return fd.pd.writeLock()
}

func (fd *netFD) readLock() error {
	return fd.pd.readLock()
}

func (fd *netFD) writeUnlock() error {
	return fd.pd.writeUnlock()
}

func (fd *netFD) readUnlock() error {
	return fd.pd.readUnlock()
}

func (fd *netFD) Close() {
	fd.pd.Close()
	syscall.Close(fd.sysfd)
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

var EpollError = errors.New("Error from epoll")

type Proxy struct {
	fd         int
	connsMutex sync.RWMutex
	conns      map[int]*netFD
}

func newProxy() *Proxy {
	fd, err := syscall.EpollCreate(10)
	if err != nil {
		return nil
	}
	p := &Proxy{
		fd:    fd,
		conns: make(map[int]*netFD, 10000),
	}
	go p.eventLoop()
	return p
}

func (p *Proxy) addFD(nfd *netFD) {
	p.connsMutex.Lock()
	p.conns[nfd.sysfd] = nfd
	p.connsMutex.Unlock()
}

func (p *Proxy) delFD(nfd *netFD) {
	p.connsMutex.Lock()
	if _, ok := p.conns[nfd.sysfd]; ok {
		delete(p.conns, nfd.sysfd)
	}
	p.connsMutex.Unlock()
}

func (p *Proxy) getFD(fd int) (nfd *netFD) {
	p.connsMutex.RLock()
	nfd, _ = p.conns[fd]
	p.connsMutex.RUnlock()
	return
}

func (p *Proxy) addConn(conn net.Conn) (*netFD, error) {
	// take out the fd
	file, err := conn.(*net.TCPConn).File()
	if err != nil {
		return nil, err
	}
	sysfd := int(file.Fd())
	if err = syscall.SetNonblock(sysfd, true); err != nil {
		return nil, err
	}

	// must add to connection list before add to epoll.
	nfd := &netFD{
		sysfd: sysfd,
	}
	nfd.pd.wcond.L = dummpMutex{}
	nfd.pd.rcond.L = dummpMutex{}
	p.addFD(nfd)

	// add to epoll
	err = syscall.EpollCtl(p.fd, EPOLL_CTL_ADD, sysfd, &syscall.EpollEvent{
		Fd:     int32(sysfd),
		Events: EPOLLIN | EPOLLOUT | EPOLLRDHUP | (EPOLLET & 0xffffffff),
	})
	if err != nil {
		p.delFD(nfd)
		return nil, err
	}
	return nfd, nil
}

func (p *Proxy) delConn(nfd *netFD) {
	syscall.EpollCtl(p.fd, EPOLL_CTL_DEL, nfd.sysfd, &syscall.EpollEvent{})
	p.delFD(nfd)
	nfd.Close()
}

func (p *Proxy) eventLoop() {
	var events [20]syscall.EpollEvent
	for {
		t1 := time.Now()
		n, eno := rawEpollWait(p.fd, events[:], -1)
		if eno != 0 {
			println("epoll wait failed, eno =", eno)
			break
		}
		println("epoll wait", time.Since(t1).String())
		for i := 0; i < n; i++ {
			nfd := p.getFD(int(events[i].Fd))
			if nfd == nil {
				continue
			}
			evs := events[i].Events

			if evs&(EPOLLERR|EPOLLHUP|EPOLLRDHUP) != 0 {
				nfd.Close()
				continue
			}

			if evs&EPOLLIN != 0 {
				nfd.pd.rcond.Signal()
			}

			if evs&EPOLLOUT != 0 {
				nfd.pd.wcond.Signal()
			}
		}
	}
}

func (p *Proxy) Transfer(conn1, conn2 net.Conn) error {
	nfd1, err := p.addConn(conn1)
	if err != nil {
		return err
	}
	conn1.Close()

	nfd2, err := p.addConn(conn2)
	if err != nil {
		return err
	}
	conn2.Close()

	errChan := make(chan error, 1)
	go func() {
		_, err, _ := splice(nfd2, nfd1, 1<<63-1)
		p.delConn(nfd1)
		p.delConn(nfd2)
		errChan <- err
	}()

	_, err, _ = splice(nfd1, nfd2, 1<<63-1)
	p.delConn(nfd1)
	p.delConn(nfd2)
	err2 := <-errChan

	if err != nil {
		return err
	}
	return err2
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
