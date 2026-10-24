package frame_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/frame"
)

func TestRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, frame.Write(&buf, []byte("hello")))
	require.NoError(t, frame.Write(&buf, nil))
	require.Equal(t, []byte{5, 0, 0, 0, 'h', 'e', 'l', 'l', 'o', 0, 0, 0, 0}, buf.Bytes())

	got, err := frame.Read(&buf)
	require.NoError(t, err)
	require.Equal(t, []byte("hello"), got)
	got, err = frame.Read(&buf)
	require.NoError(t, err)
	require.Empty(t, got)
	_, err = frame.Read(&buf)
	require.ErrorIs(t, err, io.EOF)
}

func TestTruncatedFrame(t *testing.T) {
	_, err := frame.Read(bytes.NewReader([]byte{5, 0, 0, 0, 'h', 'i'}))
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	_, err = frame.Read(bytes.NewReader([]byte{5, 0}))
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
}

func TestTooLarge(t *testing.T) {
	var hdr [4]byte
	binary.LittleEndian.PutUint32(hdr[:], frame.MaxSize+1)
	_, err := frame.Read(bytes.NewReader(hdr[:]))
	require.True(t, errors.Is(err, frame.ErrTooLarge))
	require.ErrorIs(t, frame.Write(io.Discard, make([]byte, frame.MaxSize+1)), frame.ErrTooLarge)
}

type countingWriter struct{ calls int }

func (c *countingWriter) Write(p []byte) (int, error) { c.calls++; return len(p), nil }

func TestWriteIsOneCall(t *testing.T) {
	w := &countingWriter{}
	require.NoError(t, frame.Write(w, []byte("abc")))
	require.Equal(t, 1, w.calls)
}
