package bbr

/*
// TODO(bassosimone): should I pass hardening flags here?
//CFLAGS: -Wall -Wextra -Werror -std=c11

// Design choice: it is simpler (for me) to roll out a few lines of embedded C
// than doing the CGO dance required to call the code from Go.

#include <sys/socket.h>
#include <linux/inet_diag.h>
#include <netinet/in.h>
#include <netinet/tcp.h>

#include <errno.h>
#include <stdint.h>
#include <string.h>

// get_bbr_info retrieves BBR info from |fd| and stores them in |bw| and
// |rtt| respectively. On success, returns zero. On failure returns a nonzero
// errno value indicating the error that occurred.
int get_bbr_info(int fd, double *bw, double *rtt) {
  union tcp_cc_info ti;
  if (bw == NULL || rtt == NULL) {
    return EINVAL;  // You passed me an invalid argument
  }
  memset(&ti, 0, sizeof(ti));
  socklen_t len = sizeof(ti);
  if (getsockopt(fd, IPPROTO_TCP, TCP_CC_INFO, &ti, &len) == -1) {
    return errno;  // Whatever libc said went wrong
  }
  // Apparently, tcp_bbr_info is the only congestion control data structure
  // to occupy five 32 bit words. Currently, in September 2018, the other two
  // data structures (i.e. Vegas and DCTCP) both occupy four 32 bit words.
  // See include/uapi/linux/inet_diag.h in torvalds/linux@bbb6189d.
  if (len != sizeof(struct tcp_bbr_info)) {
    return EINVAL;  // You passed me a socket that is not using TCP BBR
  }
  *bw = (double)((((uint64_t)ti.bbr.bbr_bw_hi) << 32) |
                 ((uint64_t)ti.bbr.bbr_bw_lo));
  *rtt = (double)ti.bbr.bbr_min_rtt;
  return 0;
}
*/
import "C"

import (
	"errors"
	"net"
	"strconv"
	"sync"
	"syscall"

	"github.com/apex/log"
)

// ErrBBRNoCachedFd is the error returned when there is no file descriptor
// corresponding to a local address in the internal cache.
var ErrBBRNoCachedFd = errors.New("No fd for the specified address")

// getfd returns the fd used by a given net.TCPConn.
func getfd(tc *net.TCPConn) (int, error) {
	// Implementation note: according to a 2013 message on golang-nuts [1], the
	// code that follows is broken because calling File() makes a socket blocking
	// so causing Go to use much more threads. However, an April, 19 2019 commit
	// on src/net/tcpsock.go apparently has removed such restriction and so now
	// (i.e. since go1.11beta1) it's safe to use the code below [2, 3].
	//
	// [1] https://grokbase.com/t/gg/golang-nuts/1349whs82r
	//
	// [2] https://github.com/golang/go/commit/60e3ebb9cba
	//
	// [3] https://github.com/golang/go/issues/24942
	file, err := tc.File()
	if err != nil {
		log.WithError(err).Warn("Cannot obtain a File from a TCPConn")
		return -1, err
	}
	// Note: casting to int is safe because a socket is int on Unix
	return int(file.Fd()), nil
}

// Enable takes in input a TCP connection, and attempts to enable the BBR
// congestion control algorithm for that connection. Returns nil in case
// of success, the error that occurred otherwise. Beware that the error might
// be ErrBBRNoSupport, in which case it's safe to continue, just knowing
// that you don't have BBR support on this platform.
func Enable(tc *net.TCPConn) error {
	fd, err := getfd(tc)
	if err != nil {
		return err
	}
	err = syscall.SetsockoptString(fd, syscall.IPPROTO_TCP,
		syscall.TCP_CONGESTION, "bbr")
	if err != nil {
		log.WithError(err).Warn("SetsockoptString() failed")
		return err
	}
	log.Info("TCP BBR has been successfully enabled")
	return nil
}

// Implementation note: the cache
// ``````````````````````````````
//
// To read BBR variables, we need a file descriptor. Obtaining it from a
// tls.Conn seems complex (AFAICT). So, we're keeping a mapping between the
// local address of a connection and its file descriptor.
//
// The good scenario is the one where: a connection is accepted, we map its
// local address to its file descriptor and, right after, we enter into HTTP
// code and retrieve back the file descriptor, removing it from the map.
//
// However, a connection may be closed between when it's accepted and when
// HTTP code kicks in. In such case, we'll keep the entry inside the map
// until the same local address is reused.
//
// Should we devise a strategy for cleaning the map? I am not sure. If we
// have a single local IPv4 and a single local IPv6 addresses from which we
// accept connections, we have a theoretical maximum of 128K entries. That
// is, the maximum size of the map is bounded to a small number, and it
// should not be such that accessing the map becomes slow.
//
// (I'd rather find a way to get the underlying net.Conn from the tls.Conn
//  and play with type assertions, so to make the case needless, TBH. I have
//  considered reflection but concluded it could be too fragile.)

var mutex sync.Mutex
var fds map[int]int = make(map[int]int)

// getport takes in input a TCP local address, |addrport|, and returns the int
// port corresponding to such address, or an error.
func getport(addrport string) (int, error) {
	_, port, err := net.SplitHostPort(addrport)
	if err != nil {
		return 0, err
	}
	rv, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return 0, err
	}
	return int(rv), nil
}

// RegisterFd takes in input a TCP connection and maps its LocalAddr() to
// the corresponding file descriptor. This is used such that, later, it is
// possible to map back the corresponding connection (most likely a WebSockets
// connection wrapping a tls.Conn connection) to the file descriptor without
// using reflection, which might break with future versions of golang. If
// we have no BBR support, we return ErrBBRNoSupport.
func RegisterFd(tc *net.TCPConn) error {
	fd, err := getfd(tc)
	if err != nil {
		return err
	}
	addrport := tc.LocalAddr().String()
	port, err := getport(addrport)
	if err != nil {
		return err
	}
	mutex.Lock()
	defer mutex.Unlock()
	fds[port] = fd
	return nil
}

// ExtractFd checks whether there is a file descriptor corresponding to the
// provided address. If there is one, such file descriptor will be removed from
// the internal maps and returned. Otherwise ErrBBRNoFd is returned and the
// returned file descriptor will be set to -1 in this case. If there is no
// support for BBR, instead, ErrBBRNoSupport is returned.
func ExtractFd(addrport string) (int, error) {
	port, err := getport(addrport)
	if err != nil {
		return err
	}
	mutex.Lock()
	defer mutex.Unlock()
	fd, ok := fds[port]
	if !ok {
		return -1, ErrBBRNoCachedFd
	}
	delete(fds, addr)
	return fd, nil
}

// GetBandwidthAndRTT obtains BBR info from |fd|. The returned values are the
// max-bandwidth in bytes/s and the min-rtt in microseconds. The returned
// error is ErrBBRNoSupport if BBR is not supported on this platform.
func GetBandwidthAndRTT(fd int) (float64, float64, error) {
	// Implementation note: for simplicity I have decided to use float64 here
	// rather than uint64, mainly because the proper C type to use AFAICT (and
	// I may be wrong here) changes between 32 and 64 bit. That is, it is not
	// clear to me how to use a 64 bit integer (which I what I would have used
	// by default) on a 32 bit system. So let's use float64.
	bw := C.double(0)
	rtt := C.double(0)
	rv := C.get_bbr_info(C.int(fd), &bw, &rtt)
	if rv != 0 {
		log.Warnf("C.get_bbr_info() failed; errno=%d", rv)
		return 0, 0, syscall.Errno(rv)
	}
	return float64(bw), float64(rtt), nil
}
