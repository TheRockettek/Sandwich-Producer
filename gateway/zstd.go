package gateway

import (
	"io"

	"github.com/valyala/gozstd"
)

// Compressor is something that can de/compress data
type Compressor interface {
	Compress([]byte) []byte
	Decompress([]byte) ([]byte, error)
}

// Zstd represents a de/compression context. Zero value is not valid.
type Zstd struct {
	cw *gozstd.Writer
	cr *ChanWriter
	dw io.Writer
	dr *ChanWriter
}

// NewZstd creates a valid zstd context
func NewZstd() *Zstd {
	cr := &ChanWriter{make(chan []byte)}
	zw := gozstd.NewWriter(cr)

	dr, dw := io.Pipe()
	zr := gozstd.NewReader(dr)
	dChanWriter := &ChanWriter{make(chan []byte)}
	go zr.WriteTo(dChanWriter)
	return &Zstd{zw, cr, dw, dChanWriter}
}

// Compress compresses the given bytes and returns the compressed form
func (z *Zstd) Compress(d []byte) []byte {
	z.cw.Write(d)
	go z.cw.Flush()
	return <-z.cr.C
}

// Decompress decompresses the given bytes and returns the decompressed form
func (z *Zstd) Decompress(d []byte) ([]byte, error) {
	println("Write")
	_, err := z.dw.Write(d)
	if err != nil {
		return []byte{}, err
	}

	println("Waiting :)")
	return <-z.dr.C, nil
}

// ChanWriter is a writer than sends all writes to a channel
type ChanWriter struct {
	C chan []byte
}

func (w *ChanWriter) Write(d []byte) (int, error) {
	w.C <- d
	return len(d), nil
}

// Close this writer
func (w *ChanWriter) Close() error {
	close(w.C)
	return nil
}
