// Copyright 2015 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build linux

package lab04

import (
	"io"
	"os"
	"syscall"
)

const (

	// http://linux.die.net/man/2/splice

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

func splice(dst *netFD, src *netFD, amt int64) (int64, error, bool) {
	if err := dst.writeLock(); err != nil {
		return 0, err, true
	}
	defer dst.writeUnlock()
	if err := src.readLock(); err != nil {
		return 0, err, true
	}
	defer src.readUnlock()

	var sp splicePipe
	if err := sp.init(amt); err != nil {
		// In the event that pipe2 isn't
		// supported, bail out.
		return 0, err, err != syscall.ENOSYS
	}

	var err error
	for err == nil && sp.toread != 0 {
		err = sp.readFrom(src)
		if err != nil {
			break
		}
		err = sp.writeTo(dst)
		if err != nil {
			break
		}
	}
	err1 := sp.flush(dst)
	if err == nil {
		err = err1
	}
	if err != nil {
		err = os.NewSyscallError("splice", err)
	}
	closerr := sp.destroy()
	if err == nil {
		err = closerr
	}
	return sp.written, err, true
}

type splicePipe struct {
	toread  int64 // bytes to read (<0 if unlimited)
	written int64 // bytes written
	rfd     int   // read fd
	wfd     int   // write fd
	inbuf   int64 // bytes in pipe
}

// init opens the pipe and sets the max read counter
func (s *splicePipe) init(max int64) error {
	var pipefd [2]int
	err := syscall.Pipe2(pipefd[:], syscall.O_CLOEXEC|syscall.O_NONBLOCK)
	if err != nil {
		return err
	}
	s.rfd = pipefd[0]
	s.wfd = pipefd[1]
	s.toread = max
	return nil
}

func (s *splicePipe) destroy() error {
	err := syscall.Close(s.rfd)
	err1 := syscall.Close(s.wfd)
	if err == nil {
		return err1
	}
	return err
}

// readFrom tries to splice data from 'src'
// into the pipe, but may no-op if the pipe is
// full or the read limit has been reached.
func (s *splicePipe) readFrom(src *netFD) error {
	if s.toread == 0 {
		return nil
	}

	amt := pipeBuf - s.inbuf
	if s.toread > 0 && s.toread < amt {
		amt = s.toread
	}

	// we have to differentiate
	// between blocking on read(socket)
	// and write(pipe), since both can
	// return EAGAIN
	canRead := false
read:
	// The socket->pipe splice *must* be SPLICE_F_NONBLOCK,
	// because if the pipe write blocks, then we'll deadlock.
	// n, eno := rawsplice(src.sysfd, s.wfd, int(amt), fSpliceMove|fSpliceMore|fSpliceNonblock)
	// disable fSpliceMore for response time
	n, eno := rawsplice(src.sysfd, s.wfd, int(amt), fSpliceMove|fSpliceNonblock)
	if eno == syscall.EAGAIN {
		if canRead {
			// we must be blocking b/c
			// the pipe is full
			return nil
		}
		if err := src.pd.WaitRead(); err != nil {
			return err
		}
		canRead = true
		goto read
	}

	// EOF
	if n == 0 && eno == 0 {
		if s.toread < 0 {
			s.toread = 0
			return nil
		} else {
			return io.ErrUnexpectedEOF
		}
	}

	var err error
	if eno != 0 {
		err = eno
	}

	s.inbuf += n
	s.toread -= n
	return err
}

// writeTo attempts to splice data from
// the pipe into 'dst,' but may no-op
// if there is no data in the pipe.
func (s *splicePipe) writeTo(dst *netFD) error {
	if s.inbuf == 0 {
		return nil
	}

	// we don't need to set SPLICE_F_NONBLOCK,
	// because if there's data in the pipe, then
	// EAGAIN will only occur if the socket would block
	flags := fSpliceMove
	// if we have more data to write, hint w/ SPLICE_F_MORE
	// disable fSpliceMore for response time
	//	if s.toread != 0 {
	//		flags |= fSpliceMore
	//	}

write:
	n, eno := rawsplice(s.rfd, dst.sysfd, int(s.inbuf), flags)
	if eno == syscall.EAGAIN {
		if err := dst.pd.WaitWrite(); err != nil {
			return err
		}
		goto write
	}
	var err error
	if eno != 0 {
		err = eno
	}
	s.inbuf -= n
	s.written += n
	return err
}

func (s *splicePipe) flush(dst *netFD) error {
	for s.inbuf > 0 {
		if err := s.writeTo(dst); err != nil {
			return err
		}
	}
	return nil
}

func rawsplice(srcfd int, dstfd int, amt int, flags int) (int64, syscall.Errno) {
	r, _, e := syscall.RawSyscall6(syscall.SYS_SPLICE, uintptr(srcfd), 0, uintptr(dstfd), 0, uintptr(amt), uintptr(flags))
	return int64(r), e
}
