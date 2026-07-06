//go:build !js

package host

import (
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

// stdioAddr is the placeholder net.Addr for the stdio transport — there is
// no network address, the "connection" is the process's own pipe pair.
type stdioAddr struct{}

func (stdioAddr) Network() string { return "stdio" }
func (stdioAddr) String() string  { return "stdio" }

// stdioConn adapts a read/write pipe pair — the process's stdin and stdout
// in `kapi engine serve --stdio` — into a net.Conn for the gRPC HTTP/2
// server. Deadlines are accepted and ignored (they return nil): grpc-go
// only arms a conn deadline to bound the initial connection handshake and
// clears it immediately after, and the established connection is policed by
// gRPC's own keepalive timers, so functional deadlines add nothing on a
// trusted local pipe.
type stdioConn struct {
	in  io.ReadCloser
	out io.WriteCloser

	once   sync.Once
	closed chan struct{}
}

func newStdioConn(in io.ReadCloser, out io.WriteCloser) *stdioConn {
	return &stdioConn{in: in, out: out, closed: make(chan struct{})}
}

func (c *stdioConn) Read(p []byte) (int, error)  { return c.in.Read(p) }
func (c *stdioConn) Write(p []byte) (int, error) { return c.out.Write(p) }

// Close closes both pipe ends exactly once and signals the listener that
// the session is over. gRPC closes the conn when the peer hangs up (stdin
// EOF), which is what turns "client went away" into server shutdown.
func (c *stdioConn) Close() error {
	var errIn, errOut error
	c.once.Do(func() {
		errIn = c.in.Close()
		errOut = c.out.Close()
		close(c.closed)
	})
	return errors.Join(errIn, errOut)
}

func (c *stdioConn) LocalAddr() net.Addr  { return stdioAddr{} }
func (c *stdioConn) RemoteAddr() net.Addr { return stdioAddr{} }

func (c *stdioConn) SetDeadline(time.Time) error      { return nil }
func (c *stdioConn) SetReadDeadline(time.Time) error  { return nil }
func (c *stdioConn) SetWriteDeadline(time.Time) error { return nil }

// stdioListener is a one-shot net.Listener: the first Accept hands out the
// stdio connection, and every later Accept blocks until either that
// connection or the listener itself is closed, then reports net.ErrClosed.
// grpc.Server.Serve therefore serves exactly one connection and returns
// when stdin reaches EOF (the conn closes) or the server stops (the
// listener closes).
type stdioListener struct {
	conn *stdioConn

	acceptOnce sync.Once
	closeOnce  sync.Once
	closedCh   chan struct{}
}

func newStdioListener(in io.ReadCloser, out io.WriteCloser) *stdioListener {
	return &stdioListener{conn: newStdioConn(in, out), closedCh: make(chan struct{})}
}

func (l *stdioListener) Accept() (net.Conn, error) {
	first := false
	l.acceptOnce.Do(func() { first = true })
	if first {
		return l.conn, nil
	}
	select {
	case <-l.conn.closed:
	case <-l.closedCh:
	}
	return nil, net.ErrClosed
}

// Close unblocks a pending Accept. It deliberately leaves the accepted
// connection alone: after Accept the grpc.Server owns the conn and closes
// it itself (GracefulStop drains in-flight RPCs first).
func (l *stdioListener) Close() error {
	l.closeOnce.Do(func() { close(l.closedCh) })
	return nil
}

func (l *stdioListener) Addr() net.Addr { return stdioAddr{} }
