// Package frame reads and writes the sidecar's stdio frames: a u32 little-endian length, then
// that many bytes of protobuf.
package frame

import (
	"encoding/binary"
	"errors"
	"io"
)

// MaxSize bounds one frame's payload. Media never travels in frames (DownloadMedia writes a file).
const MaxSize = 16 << 20

var ErrTooLarge = errors.New("frame: payload exceeds MaxSize")

// Write sends header and payload in one Write, so concurrent writers can't interleave a frame.
func Write(w io.Writer, payload []byte) error {
	if len(payload) > MaxSize {
		return ErrTooLarge
	}
	buf := make([]byte, 4+len(payload))
	binary.LittleEndian.PutUint32(buf, uint32(len(payload)))
	copy(buf[4:], payload)
	_, err := w.Write(buf)
	return err
}

// Read returns io.EOF only when r ends cleanly before a header.
func Read(r io.Reader) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err // io.EOF (clean) or io.ErrUnexpectedEOF (partial header)
	}
	n := binary.LittleEndian.Uint32(hdr[:])
	if n > MaxSize {
		return nil, ErrTooLarge
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return nil, err
	}
	return buf, nil
}
