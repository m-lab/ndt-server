// Package fdcache contains a mechanism to obtain the file descriptor bound
// to a websocket.Conn. We initially implemented this mechanism for BBR hence
// the following explanation is mainly tailored on the BBR case. But we also
// have other use cases for this feature in tree, e.g. reading TCP_INFO.
//
// To read BBR variables, we need a file descriptor. When serving a WebSocket
// client we have a websocket.Conn instance. The UnderlyingConn() method allows
// us to get the corresponding net.Conn, which typically is a tls.Conn. Yet,
// obtaining a file descriptor from a tls.Conn seems complex, because the
// underlying socket connection is private. Still, we've a custom listener that
// is required to turn on BBR (see tcpListenerEx). At that point, we can
// obtain a *os.File from the *net.TCPConn. From such *os.File, we can then
// get a file descriptor. However, there are some complications:
//
// a) the returned *os.File is bound to another file descriptor that is
//    a dup() of the one inside the *net.Conn, so, we should keep that
//    *os.File alive during the whole ndt7 measurement;
//
// b) in Go < 1.11, using this technique makes the file descriptor use
//    blocking I/O, which spawns more threads (see below).
//
// For these reasons, we're keeping a cache mapping between the socket's four
// tuple (i.e. local and remote address and port) and the *os.File. We use the
// four tuple because, in principle, a server can be serving on more than a
// single local IP, so only using the remote endpoint may not be enough.
//
// In the good case, this is what is gonna happen:
//
// 1. a connection is accepted in tcpListenerEx, so we have a *net.TCPConn;
//
// 2. using the *net.Conn, we turn on BBR and cache the *os.File using
//    bbr.EnableAndRememberFile() with the four tuple as the key;
//
// 3. WebSocket negotiation is successful, so we have a websocket.Conn, from
//    which we can get the underlying connection and hence the four tuple;
//
// 4. using the four tuple, we can retrieve the *os.File, removing it from
//    the cache using bbr.GetAndForgetFile();
//
// 5. we defer *os.File.Close() until the end of the WebSocket serving loop and
//    periodically we use such file to obtain the file descriptor and read the
//    BBR variables using bbr.GetMaxBandwidthAndMinRTT().
//
// Because a connection might be closed between steps 2. and 3. (i.e. after
// the connection is accepted and before the HTTP layer finishes reading the
// request and determines that it should be routed to the handler that we
// have configured), we also need a stale entry management mechanism so that
// we delete *os.File instances cached for too much time.
//
// Depending on whether Golang calls shutdown() when a socket is closed or
// not, it might be that this caching mechanism keeps connections alive for
// more time than expected. The specific case where we can have this issue
// is the one where we receive a HTTP connection that is not a valid UPGRADE
// request, but a valid HTTP request. To avoid this issue, we SHOULD make
// sure to remove the *os.File from the cache basically everytime we got our
// handler called, regardless of whether the request is a valid UPGRADE.
package fdcache

import (
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/m-lab/go/uuid"
)

// connKey is the key associated to a TCP connection.
type connKey string

// makekey creates a connKey from |conn|.
func makekey(conn net.Conn) connKey {
	return connKey(conn.LocalAddr().String() + "<=>" + conn.RemoteAddr().String())
}

// entry is an entry inside the cache.
type entry struct {
	Fp    *os.File
	Stamp time.Time
}

// cache maps a connKey to the corresponding *os.File.
var cache = make(map[connKey]entry)

// mutex serializes access to cache.
var mutex sync.Mutex

// lastCheck is last time when we checked the cache for stale entries.
var lastCheck time.Time

// checkInterval is the interval between each check for stale entries.
const checkInterval = 500 * time.Millisecond

// maxInactive is the amount of time after which an entry is stale.
const maxInactive = 3 * time.Second

// TCPConnToFile maps |tc| to the corresponding *os.File. Note that the
// returned *os.File is a dup() of the original, hence you now have ownership
// of two objects that you need to remember to defer Close() of.
func TCPConnToFile(tc *net.TCPConn) (*os.File, error) {
	// Implementation note: according to a 2013 message on golang-nuts [1], the
	// code that follows is broken on Unix because calling File() makes the socket
	// blocking so causing Go to use more threads and, additionally, "timer wheel
	// inside net package never fires". However, an April, 19 2018 commit
	// on src/net/tcpsock.go apparently has removed such restriction and so now
	// (i.e. since go1.11beta1) it's safe to use the code below [2, 3].
	//
	// [1] https://grokbase.com/t/gg/golang-nuts/1349whs82r
	//
	// [2] https://github.com/golang/go/commit/60e3ebb9cba
	//
	// [3] https://github.com/golang/go/issues/24942
	//
	// For this reason, this code only works correctly with go >= 1.11.
	return tc.File()
}

// OwnFile transfers ownership of |fp| to the fdcache module. Passing a nil
// |fp| to this function is a programming error that will cause a panic.
func OwnFile(conn net.Conn, fp *os.File) {
	if fp == nil {
		panic("You passed me a nil *os.File")
	}
	curTime := time.Now()
	key := makekey(conn)
	mutex.Lock()
	defer mutex.Unlock()
	if curTime.Sub(lastCheck) > checkInterval {
		lastCheck = curTime
		// Note: in Golang it's safe to remove elements from the map while
		// iterating it. See <https://github.com/golang/go/issues/9926>.
		for key, entry := range cache {
			if curTime.Sub(entry.Stamp) > maxInactive {
				entry.Fp.Close()
				delete(cache, key)
			}
		}
	}
	cache[key] = entry{
		Fp:    fp, // This takes ownership of fp
		Stamp: curTime,
	}
}

// GetAndForgetFile returns the *os.File bound to |conn| that was previously
// saved with EnableAndRememberFile, or nil if no file was found. Note that you
// are given ownership of the returned file pointer. As the name implies, the
// *os.File is removed from the cache by this operation.
func GetAndForgetFile(conn net.Conn) *os.File {
	key := makekey(conn)
	mutex.Lock()
	defer mutex.Unlock()
	entry, found := cache[key]
	if !found {
		return nil
	}
	delete(cache, key)
	return entry.Fp // Pass ownership to caller
}

// GetUUID returns the UUID for a passed-in connection.
func GetUUID(conn net.Conn) (string, error) {
	key := makekey(conn)
	mutex.Lock()
	defer mutex.Unlock()
	entry, found := cache[key]
	if !found {
		return "", fmt.Errorf("fd not found")
	}
	id, err := uuid.FromFile(entry.Fp)
	log.Println("UUID:", id)
	return id, err
}
