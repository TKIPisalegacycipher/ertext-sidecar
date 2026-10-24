package server

import (
	"context"
	"io"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/frame"
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
	require.NoError(t, frame.Write(inW, []byte{0xff, 0xff, 0xff})) // not a valid Envelope
	require.Error(t, <-done)
}

func TestEOFDisconnectsClient(t *testing.T) {
	f := newFake()
	h := start(t, f.factory())
	h.send(&pb.Request{Id: 1, Body: &pb.Request_Connect{Connect: &pb.Connect{SessionBlob: []byte("s")}}})
	require.NotNil(t, h.response(1).GetAck())
	require.NoError(t, h.close())
	require.Contains(t, f.Calls(), "disconnect")
}
