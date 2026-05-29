// Package compression provides zstd compression for sync protocol chunks (Bowrain AD-009).
// Uses encoder/decoder pools for zero-allocation reuse across requests.
package compression

import (
	"bytes"
	"io"
	"sync"

	"github.com/klauspost/compress/zstd"
)

// Pool provides reusable zstd encoders and decoders.
// Safe for concurrent use.
type Pool struct {
	encoders sync.Pool
	decoders sync.Pool
}

// NewPool creates a compression pool. If dict is non-nil, it is used as a
// pre-trained dictionary for better compression of repetitive data.
func NewPool(dict []byte) *Pool {
	p := &Pool{}

	var encOpts []zstd.EOption
	encOpts = append(encOpts, zstd.WithEncoderLevel(zstd.SpeedDefault))
	if len(dict) > 0 {
		encOpts = append(encOpts, zstd.WithEncoderDict(dict))
	}

	var decOpts []zstd.DOption
	if len(dict) > 0 {
		decOpts = append(decOpts, zstd.WithDecoderDicts(dict))
	}

	p.encoders = sync.Pool{
		New: func() any {
			enc, _ := zstd.NewWriter(nil, encOpts...)
			return enc
		},
	}
	p.decoders = sync.Pool{
		New: func() any {
			dec, _ := zstd.NewReader(nil, decOpts...)
			return dec
		},
	}

	return p
}

// Compress compresses data using zstd.
func (p *Pool) Compress(data []byte) []byte {
	enc := p.encoders.Get().(*zstd.Encoder)
	defer p.encoders.Put(enc)

	var buf bytes.Buffer
	enc.Reset(&buf)
	_, _ = enc.Write(data)
	_ = enc.Close()
	return buf.Bytes()
}

// Decompress decompresses zstd data.
func (p *Pool) Decompress(data []byte) ([]byte, error) {
	dec := p.decoders.Get().(*zstd.Decoder)
	defer p.decoders.Put(dec)

	return dec.DecodeAll(data, nil)
}

// CompressReader returns a reader that compresses on the fly.
func (p *Pool) CompressReader(r io.Reader) io.Reader {
	pr, pw := io.Pipe()
	enc := p.encoders.Get().(*zstd.Encoder)
	go func() {
		enc.Reset(pw)
		_, err := io.Copy(enc, r)
		_ = enc.Close()
		p.encoders.Put(enc)
		pw.CloseWithError(err)
	}()
	return pr
}
