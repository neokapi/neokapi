//go:build js && wasm

package storage

import (
	"context"
	"database/sql/driver"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall/js"
	"time"
)

// This file is the database/sql driver over the SQLite bridge
// (packages/engine/src/sqlite.ts). Arguments travel to JavaScript in one
// packed byte buffer per statement and result rows come back 256 at a time in
// another, each value a tag byte and its payload:
//
//	0 NULL
//	1 int64, little-endian
//	2 float64, little-endian
//	3 text: uint32 length, UTF-8 bytes
//	4 blob: uint32 length, bytes
//
// The row buffer starts with two uint32s: the rows it holds and whether the
// statement is exhausted. The two sides change together.

const (
	tagNull  = 0
	tagInt   = 1
	tagFloat = 2
	tagText  = 3
	tagBlob  = 4
)

// rowsPerCall is the most rows one bridge call steps. The bridge also ends a
// batch once it passes about 1 MiB, so large rows cross a few at a time.
const rowsPerCall = 256

// SQLite's result codes for a database that cannot be opened and a file that
// is not a database.
const (
	sqliteCantOpen = 14
	sqliteNotADB   = 26
)

// errNoBridge reports a host that started Go without installing the bridge.
var errNoBridge = errors.New("storage: the SQLite bridge (globalThis.__kapiSQL) is not installed; " +
	"the host loads @sqlite.org/sqlite-wasm and installs it before the Go program starts (packages/engine)")

// jsError is a failure the SQLite WebAssembly build reported: its message and
// extended result code. The message is SQLite's own, as the native drivers
// report it.
type jsError struct {
	code int
	msg  string
}

func (e *jsError) Error() string { return e.msg }

// bridgeState is the Go side of the bridge: the JavaScript object, the shared
// argument buffer, and the lock that keeps one goroutine's pack-call-unpack
// sequence from interleaving with another's.
type bridgeState struct {
	mu    sync.Mutex
	obj   js.Value
	io    js.Value
	ioLen int
	args  []byte
}

var bridge bridgeState

// sqlBridge returns the installed bridge object.
func sqlBridge() (js.Value, error) {
	if bridge.obj.IsUndefined() || bridge.obj.IsNull() {
		obj := js.Global().Get("__kapiSQL")
		if obj.Type() != js.TypeObject {
			return js.Value{}, errNoBridge
		}
		bridge.obj = obj
	}
	return bridge.obj, nil
}

// bridgeCall calls a bridge method and turns a `{err, code}` answer into an
// error.
func bridgeCall(method string, args ...any) (js.Value, error) {
	b, err := sqlBridge()
	if err != nil {
		return js.Value{}, err
	}
	r := b.Call(method, args...)
	if r.Type() == js.TypeObject {
		if e := r.Get("err"); e.Type() == js.TypeString {
			return r, &jsError{code: r.Get("code").Int(), msg: e.String()}
		}
	}
	return r, nil
}

// putArgs packs args into the shared buffer and returns its length. The
// caller holds bridge.mu.
func putArgs(args []driver.NamedValue) (int, error) {
	b := bridge.args[:0]
	for _, a := range args {
		if a.Name != "" {
			return 0, fmt.Errorf("storage: named parameter %q: the browser driver binds by position", a.Name)
		}
		switch v := a.Value.(type) {
		case nil:
			b = append(b, tagNull)
		case int64:
			b = append(b, tagInt)
			b = binary.LittleEndian.AppendUint64(b, uint64(v))
		case bool:
			b = append(b, tagInt)
			var n uint64
			if v {
				n = 1
			}
			b = binary.LittleEndian.AppendUint64(b, n)
		case float64:
			b = append(b, tagFloat)
			b = binary.LittleEndian.AppendUint64(b, math.Float64bits(v))
		case string:
			b = append(b, tagText)
			b = binary.LittleEndian.AppendUint32(b, uint32(len(v)))
			b = append(b, v...)
		case []byte:
			if v == nil {
				b = append(b, tagNull)
				break
			}
			b = append(b, tagBlob)
			b = binary.LittleEndian.AppendUint32(b, uint32(len(v)))
			b = append(b, v...)
		default:
			return 0, fmt.Errorf("storage: cannot bind %T", v)
		}
	}
	bridge.args = b
	if err := ensureIO(); err != nil {
		return 0, err
	}
	if bridge.ioLen < len(b) {
		size := bridge.ioLen * 2
		for size < len(b) {
			size *= 2
		}
		bridge.io = js.Global().Get("Uint8Array").New(size)
		bridge.ioLen = size
		bridge.obj.Set("io", bridge.io)
	}
	if len(b) > 0 {
		js.CopyBytesToJS(bridge.io, b)
	}
	return len(b), nil
}

// readResult reads the change count and last row id a run or exec wrote. The
// caller holds bridge.mu.
func readResult() driver.Result {
	var out [16]byte
	js.CopyBytesToGo(out[:], bridge.io)
	return jsResult{
		changes: int64(binary.LittleEndian.Uint64(out[0:])),
		rowid:   int64(binary.LittleEndian.Uint64(out[8:])),
	}
}

// ensureIO makes sure the shared buffer exists before a call that writes a
// result into it. The caller holds bridge.mu.
func ensureIO() error {
	if _, err := sqlBridge(); err != nil {
		return err
	}
	if bridge.io.IsUndefined() {
		bridge.io = bridge.obj.Get("io")
		bridge.ioLen = bridge.io.Length()
		if bridge.ioLen < 16 {
			return errors.New("storage: the SQLite bridge has no argument buffer")
		}
	}
	return nil
}

type jsResult struct{ changes, rowid int64 }

func (r jsResult) LastInsertId() (int64, error) { return r.rowid, nil }
func (r jsResult) RowsAffected() (int64, error) { return r.changes, nil }

// jsDriver opens connections through the bridge.
type jsDriver struct{}

// Open takes the DSN sqliteDSN builds: a database name, then
// `_pragma=NAME(VALUE)` parameters run once on the new connection and an
// optional `_txlock` naming how its transactions begin.
func (jsDriver) Open(dsn string) (driver.Conn, error) {
	name, query, _ := strings.Cut(strings.TrimPrefix(dsn, "file:"), "?")
	// A database is held in memory, but it is named by a path in the file
	// system the program sees, and a path whose directory is not there fails
	// as it does natively rather than succeeding only in the browser.
	if strings.HasPrefix(name, "/") {
		if info, err := os.Stat(filepath.Dir(name)); err != nil || !info.IsDir() {
			return nil, &jsError{code: sqliteCantOpen, msg: "unable to open database file"}
		}
		if err := readFileIn(name); err != nil {
			return nil, err
		}
	}
	bridge.mu.Lock()
	r, err := bridgeCall("open", name)
	bridge.mu.Unlock()
	if err != nil {
		return nil, err
	}
	c := &jsConn{id: r.Int()}
	q, perr := url.ParseQuery(query)
	if perr != nil {
		_ = c.Close()
		return nil, fmt.Errorf("storage: parse DSN parameters: %w", perr)
	}
	for _, p := range q["_pragma"] {
		pragma := "PRAGMA " + strings.Replace(strings.TrimSuffix(p, ")"), "(", "=", 1)
		if _, err := c.exec(pragma); err != nil {
			_ = c.Close()
			return nil, fmt.Errorf("storage: %s: %w", pragma, err)
		}
	}
	switch lock := strings.ToUpper(q.Get("_txlock")); lock {
	case "", "DEFERRED", "IMMEDIATE", "EXCLUSIVE":
		c.txlock = lock
	default:
		_ = c.Close()
		return nil, fmt.Errorf("storage: unknown _txlock %q", lock)
	}
	return c, nil
}

// jsConn is one connection the bridge holds open.
type jsConn struct {
	id     int
	txlock string
	closed bool
}

var (
	_ driver.ExecerContext      = (*jsConn)(nil)
	_ driver.QueryerContext     = (*jsConn)(nil)
	_ driver.ConnBeginTx        = (*jsConn)(nil)
	_ driver.ConnPrepareContext = (*jsConn)(nil)
	_ driver.NamedValueChecker  = (*jsConn)(nil)
	_ driver.Validator          = (*jsConn)(nil)
)

func (c *jsConn) IsValid() bool { return !c.closed }

func (c *jsConn) Close() error {
	if c.closed {
		return nil
	}
	c.closed = true
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	_, err := bridgeCall("close", c.id)
	return err
}

// exec runs SQL with no arguments, any number of statements.
func (c *jsConn) exec(query string) (driver.Result, error) {
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	if err := ensureIO(); err != nil {
		return nil, err
	}
	if _, err := bridgeCall("exec", c.id, query); err != nil {
		return nil, err
	}
	return readResult(), nil
}

func (c *jsConn) prepare(query string) (*jsStmt, error) {
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	r, err := bridgeCall("prepare", c.id, query)
	if err != nil {
		return nil, err
	}
	s := &jsStmt{
		conn:   c,
		id:     r.Get("id").Int(),
		params: r.Get("params").Int(),
		tail:   r.Get("tail").String(),
	}
	cols, decl := r.Get("cols"), r.Get("decl")
	s.cols = make([]string, cols.Length())
	s.decl = make([]string, decl.Length())
	for i := range s.cols {
		s.cols[i] = cols.Index(i).String()
		s.decl[i] = decl.Index(i).String()
	}
	return s, nil
}

func (c *jsConn) Prepare(query string) (driver.Stmt, error) {
	return c.PrepareContext(context.Background(), query)
}

// PrepareContext compiles the first statement in query, as the native drivers
// do for a prepared statement.
func (c *jsConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return c.prepare(query)
}

func (c *jsConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *jsConn) BeginTx(ctx context.Context, _ driver.TxOptions) (driver.Tx, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	begin := "BEGIN"
	if c.txlock != "" {
		begin += " " + c.txlock
	}
	if _, err := c.exec(begin); err != nil {
		return nil, err
	}
	return jsTx{c: c}, nil
}

// ExecContext runs every statement in query, handing each the arguments its
// parameters take in order, as mattn/go-sqlite3 does.
func (c *jsConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(args) == 0 {
		return c.exec(query)
	}
	var res driver.Result = jsResult{}
	for query != "" {
		s, err := c.prepare(query)
		if err != nil {
			return nil, err
		}
		if s.id != 0 {
			if len(args) < s.params {
				_ = s.Close()
				return nil, fmt.Errorf("storage: not enough arguments: want %d, got %d", s.params, len(args))
			}
			res, err = s.run(args[:s.params])
			args = args[s.params:]
			_ = s.Close()
			if err != nil {
				return nil, err
			}
		}
		query = s.tail
	}
	return res, nil
}

// QueryContext runs every statement before the last for its effect and
// returns the rows of the last, as mattn/go-sqlite3 does.
func (c *jsConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for {
		s, err := c.prepare(query)
		if err != nil {
			return nil, err
		}
		if len(args) < s.params {
			_ = s.Close()
			return nil, fmt.Errorf("storage: not enough arguments: want %d, got %d", s.params, len(args))
		}
		stmtArgs := args[:s.params]
		args = args[s.params:]
		if s.tail == "" || s.id == 0 {
			if s.id == 0 {
				return &jsRows{s: s, done: true, closeStmt: true}, nil
			}
			rows, err := s.query(stmtArgs)
			if err != nil {
				_ = s.Close()
				return nil, err
			}
			rows.closeStmt = true
			return rows, nil
		}
		_, err = s.run(stmtArgs)
		_ = s.Close()
		if err != nil {
			return nil, err
		}
		query = s.tail
	}
}

// CheckNamedValue writes a time.Time in mattn/go-sqlite3's text form, so a
// database written in the browser compares with one written natively, and
// leaves every other value to database/sql's default conversion.
func (c *jsConn) CheckNamedValue(nv *driver.NamedValue) error {
	if t, ok := nv.Value.(time.Time); ok {
		nv.Value = t.Format(timestampFormats[0])
		return nil
	}
	return driver.ErrSkip
}

type jsTx struct{ c *jsConn }

// Commit runs COMMIT, and ROLLBACK when COMMIT fails, as mattn/go-sqlite3
// does. SQLite keeps the transaction open after a failed COMMIT (SQLITE_BUSY
// while another connection reads the database), but database/sql treats the
// transaction as over and returns the connection to the pool. Left open, it
// would hold the pool's only connection inside a transaction nothing commits:
// every later Begin would fail and every later write would join it.
func (t jsTx) Commit() error {
	_, err := t.c.exec("COMMIT")
	if err != nil {
		_, _ = t.c.exec("ROLLBACK")
	}
	return err
}

func (t jsTx) Rollback() error {
	_, err := t.c.exec("ROLLBACK")
	return err
}

// jsStmt is a compiled statement.
type jsStmt struct {
	conn   *jsConn
	id     int
	params int
	cols   []string
	decl   []string
	tail   string
	closed bool
}

var (
	_ driver.StmtExecContext  = (*jsStmt)(nil)
	_ driver.StmtQueryContext = (*jsStmt)(nil)
)

func (s *jsStmt) Close() error {
	if s.closed || s.id == 0 {
		s.closed = true
		return nil
	}
	s.closed = true
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	_, err := bridgeCall("finalize", s.id)
	return err
}

func (s *jsStmt) NumInput() int { return s.params }

func (s *jsStmt) Exec(args []driver.Value) (driver.Result, error) {
	return s.ExecContext(context.Background(), namedValues(args))
}

func (s *jsStmt) Query(args []driver.Value) (driver.Rows, error) {
	return s.QueryContext(context.Background(), namedValues(args))
}

func (s *jsStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.run(args)
}

func (s *jsStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.query(args)
}

func (s *jsStmt) run(args []driver.NamedValue) (driver.Result, error) {
	if s.id == 0 {
		return jsResult{}, nil
	}
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	n, err := putArgs(args)
	if err != nil {
		return nil, err
	}
	if _, err := bridgeCall("run", s.id, n); err != nil {
		return nil, err
	}
	return readResult(), nil
}

func (s *jsStmt) query(args []driver.NamedValue) (*jsRows, error) {
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	n, err := putArgs(args)
	if err != nil {
		return nil, err
	}
	if _, err := bridgeCall("query", s.id, n); err != nil {
		return nil, err
	}
	return &jsRows{s: s}, nil
}

func namedValues(args []driver.Value) []driver.NamedValue {
	out := make([]driver.NamedValue, len(args))
	for i, a := range args {
		out[i] = driver.NamedValue{Ordinal: i + 1, Value: a}
	}
	return out
}

// jsRows reads a query's rows a batch at a time.
type jsRows struct {
	s         *jsStmt
	buf       []byte
	pos       int
	left      int
	done      bool
	closeStmt bool
	closed    bool
}

var _ driver.RowsColumnTypeDatabaseTypeName = (*jsRows)(nil)

func (r *jsRows) Columns() []string { return r.s.cols }

func (r *jsRows) ColumnTypeDatabaseTypeName(i int) string { return strings.ToUpper(r.s.decl[i]) }

func (r *jsRows) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	if r.closeStmt {
		return r.s.Close()
	}
	if !r.done && r.s.id != 0 {
		bridge.mu.Lock()
		defer bridge.mu.Unlock()
		_, err := bridgeCall("reset", r.s.id)
		return err
	}
	return nil
}

// fill fetches the next batch of rows.
func (r *jsRows) fill() error {
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	res, err := bridgeCall("next", r.s.id, rowsPerCall)
	if err != nil {
		return err
	}
	n := res.Int()
	// A batch is about 1 MiB at most, unless one row is larger; a buffer
	// grown for such a row is not kept for the batches after it.
	if cap(r.buf) < n || cap(r.buf) > max(4*n, 2<<20) {
		r.buf = make([]byte, n)
	}
	r.buf = r.buf[:n]
	js.CopyBytesToGo(r.buf, bridge.obj.Get("rows"))
	r.left = int(binary.LittleEndian.Uint32(r.buf[0:]))
	r.done = binary.LittleEndian.Uint32(r.buf[4:]) != 0
	r.pos = 8
	return nil
}

func (r *jsRows) Next(dest []driver.Value) error {
	if r.left == 0 {
		if r.done {
			return io.EOF
		}
		if err := r.fill(); err != nil {
			return err
		}
		if r.left == 0 {
			return io.EOF
		}
	}
	r.left--
	b := r.buf
	for i := range dest {
		tag := b[r.pos]
		r.pos++
		decl := r.s.decl[i]
		switch tag {
		case tagInt:
			v := int64(binary.LittleEndian.Uint64(b[r.pos:]))
			r.pos += 8
			dest[i] = intValue(v, decl)
		case tagFloat:
			dest[i] = math.Float64frombits(binary.LittleEndian.Uint64(b[r.pos:]))
			r.pos += 8
		case tagText:
			n := int(binary.LittleEndian.Uint32(b[r.pos:]))
			s := string(b[r.pos+4 : r.pos+4+n])
			r.pos += 4 + n
			dest[i] = textValue(s, decl)
		case tagBlob:
			n := int(binary.LittleEndian.Uint32(b[r.pos:]))
			dest[i] = append([]byte{}, b[r.pos+4:r.pos+4+n]...)
			r.pos += 4 + n
		default:
			dest[i] = nil
		}
	}
	return nil
}

// timestampFormats are the forms mattn/go-sqlite3 writes and reads a
// time.Time in. The driver matches them, so a column declared DATE, DATETIME
// or TIMESTAMP scans into a time.Time in the browser as it does natively.
var timestampFormats = []string{
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02T15:04:05.999999999-07:00",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02T15:04:05.999999999",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04",
	"2006-01-02T15:04",
	"2006-01-02",
}

func isTimeDecl(decl string) bool {
	return decl == "date" || decl == "datetime" || decl == "timestamp"
}

// intValue converts an integer the way mattn/go-sqlite3 does for the column's
// declared type: a time column holds Unix seconds (or milliseconds, past
// 13 digits), a boolean column a truth value.
func intValue(v int64, decl string) driver.Value {
	switch {
	case isTimeDecl(decl):
		if v > 1e12 || v < -1e12 {
			return time.Unix(0, v*int64(time.Millisecond)).UTC()
		}
		return time.Unix(v, 0).UTC()
	case decl == "boolean":
		return v > 0
	default:
		return v
	}
}

// textValue converts text the way mattn/go-sqlite3 does for the column's
// declared type: a time column parses into a time.Time, the zero time when no
// format matches.
func textValue(s, decl string) driver.Value {
	if !isTimeDecl(decl) {
		return s
	}
	s = strings.TrimSuffix(s, "Z")
	for _, f := range timestampFormats {
		if t, err := time.ParseInLocation(f, s, time.UTC); err == nil {
			return t
		}
	}
	return time.Time{}
}
