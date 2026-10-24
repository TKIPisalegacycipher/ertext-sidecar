package server

import (
	"context"
	"errors"
	"io"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/frame"
	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/gm"
	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/pb"
)

func TestHelloIsFirstFrame(t *testing.T) {
	h := start(t, newFake().factory())
	env := h.next()
	// Decoded messages carry internal protoimpl state: compare with proto.Equal, not require.Equal.
	require.True(t, proto.Equal(&pb.Hello{ProtocolVersion: ProtocolVersion, SidecarVersion: "test", LibgmVersion: "libgm-test"},
		env.GetEvent().GetHello()))
	require.NoError(t, h.close())
}

func TestPing(t *testing.T) {
	h := start(t, newFake().factory())
	h.send(&pb.Request{Id: 1, Body: &pb.Request_Ping{Ping: &pb.Ping{}}})
	require.NotNil(t, h.response(1).GetPong())
	require.NoError(t, h.close())
}

func TestUnknownRequestBody(t *testing.T) {
	h := start(t, newFake().factory())
	h.send(&pb.Request{Id: 2})
	r := h.response(2)
	require.Equal(t, pb.ErrorCode_ERROR_CODE_BAD_REQUEST, r.GetError().GetCode())
	require.NoError(t, h.close())
}

func TestRequestsWithoutClientAreNotConnected(t *testing.T) {
	h := start(t, newFake().factory())
	h.send(&pb.Request{Id: 3, Body: &pb.Request_SetActive{SetActive: &pb.SetActive{}}})
	require.Equal(t, pb.ErrorCode_ERROR_CODE_NOT_CONNECTED, h.response(3).GetError().GetCode())
	require.NoError(t, h.close())
}

func TestBadFrameIsFatal(t *testing.T) {
	inR, inW := io.Pipe()
	srv := New(newFake().factory(), zerolog.Nop(), "test", "x")
	done := make(chan error, 1)
	go func() { done <- srv.Run(context.Background(), inR, io.Discard) }()
	defer inW.Close()
	require.NoError(t, frame.Write(inW, []byte{0xff, 0xff, 0xff})) // not a valid Envelope
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("server did not exit")
	}
}

func TestEOFDisconnectsClient(t *testing.T) {
	f := newFake()
	h := start(t, f.factory())
	h.send(&pb.Request{Id: 1, Body: &pb.Request_Connect{Connect: &pb.Connect{SessionBlob: []byte("s")}}})
	require.NotNil(t, h.response(1).GetAck())
	require.NoError(t, h.close())
	require.Contains(t, f.Calls(), "disconnect")
}

func TestInFlightResponseDeliveredAfterEOF(t *testing.T) {
	f := newFake()
	f.connectGate = make(chan struct{})
	h := start(t, f.factory())
	h.send(&pb.Request{Id: 1, Body: &pb.Request_Connect{Connect: &pb.Connect{SessionBlob: []byte("s")}}})
	require.Eventually(t, func() bool { return slices.Contains(f.Calls(), "connect") }, 5*time.Second, time.Millisecond)
	_ = h.in.Close() // EOF while the Connect handler is still in flight
	close(f.connectGate)
	require.NotNil(t, h.response(1).GetAck())
	require.NoError(t, h.close())
	require.Contains(t, f.Calls(), "disconnect")
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }

func TestRunReturnsOnWriteFailure(t *testing.T) {
	inR, inW := io.Pipe() // stdin stays open
	defer inW.Close()
	srv := New(newFake().factory(), zerolog.Nop(), "test", "x")
	done := make(chan error, 1)
	go func() { done <- srv.Run(context.Background(), inR, failWriter{}) }()
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after write failure")
	}
}

func TestSupersededConnectIsDisconnected(t *testing.T) {
	a, b := newFake(), newFake()
	a.connectGate = make(chan struct{})
	var mu sync.Mutex
	next := []*fakeClient{a, b}
	factory := func(session []byte) (gm.Client, error) {
		mu.Lock()
		defer mu.Unlock()
		c := next[0]
		next = next[1:]
		return c, nil
	}
	h := start(t, factory)
	h.send(&pb.Request{Id: 1, Body: &pb.Request_Connect{Connect: &pb.Connect{SessionBlob: []byte("a")}}})
	require.Eventually(t, func() bool { return slices.Contains(a.Calls(), "connect") }, 5*time.Second, time.Millisecond)
	h.send(&pb.Request{Id: 2, Body: &pb.Request_Connect{Connect: &pb.Connect{SessionBlob: []byte("b")}}})
	require.NotNil(t, h.response(2).GetAck())
	close(a.connectGate)
	require.NotNil(t, h.response(1).GetError())
	require.Contains(t, a.Calls(), "disconnect")
	require.NotContains(t, b.Calls(), "disconnect")
	require.NoError(t, h.close())
}
