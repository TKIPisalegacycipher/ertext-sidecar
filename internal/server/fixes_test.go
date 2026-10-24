package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"go.mau.fi/mautrix-gmessages/pkg/libgm"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/events"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"

	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/gm"
	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/pb"
)

func wrapped(id string, st gmproto.MessageStatusType) *libgm.WrappedMessage {
	return &libgm.WrappedMessage{Message: &gmproto.Message{
		MessageID: id, ConversationID: "c1", MessageStatus: &gmproto.MessageStatus{Status: st},
	}}
}

func alert(a gmproto.AlertType) *gmproto.UserAlertEvent { return &gmproto.UserAlertEvent{AlertType: a} }

func isDeleted(id string) func(*pb.Event) bool {
	return func(e *pb.Event) bool { return e.GetMessageDeleted().GetMessageId() == id }
}

func TestMessageDeletedSuppressedDuringMobileDBSync(t *testing.T) {
	h, f := connected(t)
	f.fire(wrapped("m1", gmproto.MessageStatusType_MESSAGE_DELETED))
	require.Equal(t, "c1", h.event(isDeleted("m1")).GetMessageDeleted().GetConversationId())

	f.fire(alert(gmproto.AlertType_MOBILE_DATABASE_SYNC_STARTED))
	f.fire(wrapped("m-sync", gmproto.MessageStatusType_MESSAGE_DELETED))   // dropped
	f.fire(wrapped("m-other", gmproto.MessageStatusType_INCOMING_DELETED)) // not 300: still sent
	f.fire(wrapped("m-new", gmproto.MessageStatusType_INCOMING_COMPLETE))  // upserts still flow
	f.fire(alert(gmproto.AlertType_MOBILE_DATABASE_SYNC_COMPLETE))         // -> ResyncNeeded
	h.event(func(e *pb.Event) bool { return e.GetResyncNeeded() != nil })  // events arrive in order
	f.fire(wrapped("m2", gmproto.MessageStatusType_MESSAGE_DELETED))       // sync over: sent again
	h.event(isDeleted("m2"))

	for _, e := range h.events {
		require.NotEqual(t, "m-sync", e.GetMessageDeleted().GetMessageId(), "MESSAGE_DELETED leaked during sync")
	}
	require.NotNil(t, h.event(isDeleted("m-other")))
	require.NotNil(t, h.event(func(e *pb.Event) bool { return e.GetMessageUpserted().GetMessage().GetId() == "m-new" }))
	require.NoError(t, h.close())
}

func TestDBSyncFlagIsPerClient(t *testing.T) {
	a, b := newFake(), newFake()
	var mu sync.Mutex
	next := []*fakeClient{a, b}
	h := start(t, func([]byte) (gm.Client, error) {
		mu.Lock()
		defer mu.Unlock()
		c := next[0]
		next = next[1:]
		return c, nil
	})
	h.send(req(1, &pb.Connect{SessionBlob: []byte("a")}))
	require.NotNil(t, h.response(1).GetAck())
	a.fire(alert(gmproto.AlertType_MOBILE_DATABASE_SYNC_STARTED)) // a never completes its sync
	h.send(req(2, &pb.Connect{SessionBlob: []byte("b")}))
	require.NotNil(t, h.response(2).GetAck())
	b.fire(wrapped("m1", gmproto.MessageStatusType_MESSAGE_DELETED))
	require.NotNil(t, h.event(isDeleted("m1")))
	require.NoError(t, h.close())
}

func httpErr(action string, code int) error {
	return events.HTTPError{Action: action, Resp: &http.Response{StatusCode: code}}
}

func TestCodeForHTTPErrors(t *testing.T) {
	for _, code := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound} {
		require.Equal(t, pb.ErrorCode_ERROR_CODE_UNAUTHORIZED, codeFor(errors.Join(errors.New("failed to refresh auth token"), httpErr("", code))), code)
		require.Equal(t, pb.ErrorCode_ERROR_CODE_UNAUTHORIZED, codeFor(httpErr("polling", code)), code)
	}
	require.Equal(t, pb.ErrorCode_ERROR_CODE_INTERNAL, codeFor(httpErr("", http.StatusInternalServerError)))
	require.Equal(t, pb.ErrorCode_ERROR_CODE_INTERNAL, codeFor(events.HTTPError{})) // nil Resp
}

func TestConnectFatalRefreshHTTPErrorIsUnauthorized(t *testing.T) {
	f := newFake()
	f.err["connect"] = httpErr("", http.StatusForbidden)
	h := start(t, f.factory())
	h.send(req(1, &pb.Connect{SessionBlob: []byte("s")}))
	require.Equal(t, pb.ErrorCode_ERROR_CODE_UNAUTHORIZED, h.response(1).GetError().GetCode())
	require.Contains(t, f.Calls(), "disconnect", "failed Connect is disconnected")
	require.NoError(t, h.close())
}

func TestStartPairingErrorsDisconnect(t *testing.T) {
	for _, step := range []string{"config", "login"} {
		t.Run(step, func(t *testing.T) {
			f := newFake()
			f.err[step] = errors.New("relay down")
			h := start(t, f.factory())
			h.send(req(1, &pb.StartPairing{}))
			require.NotNil(t, h.response(1).GetError())
			require.Contains(t, f.Calls(), "disconnect", "long poll must not leak")
			h.send(req(2, &pb.RefreshPairing{}))
			require.Equal(t, pb.ErrorCode_ERROR_CODE_NOT_CONNECTED, h.response(2).GetError().GetCode())
			require.NoError(t, h.close())
		})
	}
}

func TestUnpairKeepsNewerClient(t *testing.T) {
	a, b := newFake(), newFake()
	a.unpairGate = make(chan struct{})
	var mu sync.Mutex
	next := []*fakeClient{a, b}
	h := start(t, func([]byte) (gm.Client, error) {
		mu.Lock()
		defer mu.Unlock()
		c := next[0]
		next = next[1:]
		return c, nil
	})
	h.send(req(1, &pb.Connect{SessionBlob: []byte("a")}))
	require.NotNil(t, h.response(1).GetAck())
	h.send(req(2, &pb.Unpair{}))
	require.Eventually(t, func() bool { return slices.Contains(a.Calls(), "unpair") }, 5*time.Second, time.Millisecond)
	h.send(req(3, &pb.Connect{SessionBlob: []byte("b")})) // supersedes a mid-Unpair
	require.NotNil(t, h.response(3).GetAck())
	close(a.unpairGate)
	require.NotNil(t, h.response(2).GetAck())

	h.send(req(4, &pb.SetActive{})) // b is still current
	require.NotNil(t, h.response(4).GetAck())
	require.Contains(t, b.Calls(), "setactive")
	require.NotContains(t, b.Calls(), "disconnect")
	require.Contains(t, a.Calls(), "disconnect")
	require.NoError(t, h.close())
}

func TestRunReturnsOnContextCancel(t *testing.T) {
	inR, inW := io.Pipe() // stdin stays open
	defer inW.Close()
	f := newFake()
	srv := New(f.factory(), zerolog.Nop(), "test", "x")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx, inR, io.Discard) }()
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after ctx cancel")
	}
}

func TestRunContextCancelDisconnectsClient(t *testing.T) {
	inR, inW := io.Pipe()
	defer inW.Close()
	outR, outW := io.Pipe()
	go func() { _, _ = io.Copy(io.Discard, outR) }()
	f := newFake()
	srv := New(f.factory(), zerolog.Nop(), "test", "x")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx, inR, outW) }()
	h := &harness{t: t, in: inW}
	h.send(req(1, &pb.Connect{SessionBlob: []byte("s")}))
	require.Eventually(t, func() bool { return slices.Contains(f.Calls(), "connect") }, 5*time.Second, time.Millisecond)
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after ctx cancel")
	}
	require.Contains(t, f.Calls(), "disconnect")
}

func TestRequestHandlerPanicBecomesInternalError(t *testing.T) {
	h, f := connected(t)
	f.mu.Lock()
	f.panicOn["setactive"] = true
	f.mu.Unlock()
	h.send(req(7, &pb.SetActive{}))
	r := h.response(7)
	require.Equal(t, pb.ErrorCode_ERROR_CODE_INTERNAL, r.GetError().GetCode())
	require.NotContains(t, r.GetError().GetMessage(), "fake panic", "panic contents must not leak")
	h.send(&pb.Request{Id: 8, Body: &pb.Request_Ping{Ping: &pb.Ping{}}}) // server still serving
	require.NotNil(t, h.response(8).GetPong())
	require.NoError(t, h.close())
}

func TestEventHandlerPanicIsDropped(t *testing.T) {
	h, f := connected(t)
	f.mu.Lock()
	f.panicOn["blob"] = true
	f.mu.Unlock()
	require.NotPanics(t, func() { f.fire(&events.AuthTokenRefreshed{}) })
	f.fire(&events.PhoneNotResponding{})
	st := h.event(func(e *pb.Event) bool { return e.GetConnectionState() != nil })
	require.Equal(t, pb.ConnState_CONN_STATE_PHONE_OFFLINE, st.GetConnectionState().GetState())
	for _, e := range h.events {
		require.Nil(t, e.GetSessionUpdated())
	}
	require.NoError(t, h.close())
}
