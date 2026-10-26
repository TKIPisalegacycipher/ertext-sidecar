package server

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/events"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"

	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/pb"
)

func req(id uint64, body any) *pb.Request {
	r := &pb.Request{Id: id}
	switch b := body.(type) {
	case *pb.StartGoogleLogin:
		r.Body = &pb.Request_StartGoogleLogin{StartGoogleLogin: b}
	case *pb.StartPairing:
		r.Body = &pb.Request_StartPairing{StartPairing: b}
	case *pb.RefreshPairing:
		r.Body = &pb.Request_RefreshPairing{RefreshPairing: b}
	case *pb.Connect:
		r.Body = &pb.Request_Connect{Connect: b}
	case *pb.Disconnect:
		r.Body = &pb.Request_Disconnect{Disconnect: b}
	case *pb.Unpair:
		r.Body = &pb.Request_Unpair{Unpair: b}
	case *pb.SetActive:
		r.Body = &pb.Request_SetActive{SetActive: b}
	case *pb.ListConversations:
		r.Body = &pb.Request_ListConversations{ListConversations: b}
	case *pb.FetchMessages:
		r.Body = &pb.Request_FetchMessages{FetchMessages: b}
	case *pb.SendMessage:
		r.Body = &pb.Request_SendMessage{SendMessage: b}
	case *pb.SendReaction:
		r.Body = &pb.Request_SendReaction{SendReaction: b}
	case *pb.MarkRead:
		r.Body = &pb.Request_MarkRead{MarkRead: b}
	case *pb.ArchiveConversation:
		r.Body = &pb.Request_ArchiveConversation{ArchiveConversation: b}
	case *pb.DeleteConversation:
		r.Body = &pb.Request_DeleteConversation{DeleteConversation: b}
	case *pb.GetOrCreateConversation:
		r.Body = &pb.Request_GetOrCreateConversation{GetOrCreateConversation: b}
	case *pb.ListContacts:
		r.Body = &pb.Request_ListContacts{ListContacts: b}
	case *pb.CheckRcs:
		r.Body = &pb.Request_CheckRcs{CheckRcs: b}
	case *pb.DownloadMedia:
		r.Body = &pb.Request_DownloadMedia{DownloadMedia: b}
	case *pb.RequestFullMedia:
		r.Body = &pb.Request_RequestFullMedia{RequestFullMedia: b}
	case *pb.GetThumbnails:
		r.Body = &pb.Request_GetThumbnails{GetThumbnails: b}
	case *pb.GetContactThumbnails:
		r.Body = &pb.Request_GetContactThumbnails{GetContactThumbnails: b}
	default:
		panic(fmt.Sprintf("req: unhandled %T", body))
	}
	return r
}

func connected(t *testing.T) (*harness, *fakeClient) {
	t.Helper()
	f := newFake()
	h := start(t, f.factory())
	h.send(req(1, &pb.Connect{SessionBlob: []byte("sess")}))
	require.NotNil(t, h.response(1).GetAck())
	return h, f
}

func TestPairingFlow(t *testing.T) {
	f := newFake()
	h := start(t, f.factory())
	h.send(req(1, &pb.StartPairing{}))
	require.NotNil(t, h.response(1).GetAck())
	require.Nil(t, f.sessionArg(), "pairing starts from fresh AuthData")
	qr := h.event(func(e *pb.Event) bool { return e.GetPairQr() != nil })
	require.Equal(t, "https://qr/1", qr.GetPairQr().GetUrl())

	h.send(req(2, &pb.RefreshPairing{}))
	require.NotNil(t, h.response(2).GetAck())
	qr2 := h.event(func(e *pb.Event) bool { return e.GetPairQr() != nil })
	require.Equal(t, "https://qr/2", qr2.GetPairQr().GetUrl())

	f.fire(&events.PhoneNotResponding{}) // pairing client's events are not forwarded

	f.mu.Lock()
	cb := f.pairCB
	f.mu.Unlock()
	cb(&gmproto.PairedData{})
	ok := h.event(func(e *pb.Event) bool { return e.GetPairSuccess() != nil })
	require.Equal(t, []byte(`{"fake":true}`), ok.GetPairSuccess().GetSessionBlob())
	require.Contains(t, f.Calls(), "disconnect")

	h.send(req(3, &pb.SetActive{})) // slot is empty after pairing
	require.Equal(t, pb.ErrorCode_ERROR_CODE_NOT_CONNECTED, h.response(3).GetError().GetCode())
	for _, e := range h.events {
		require.Nil(t, e.GetConnectionState(), "pre-pairing event leaked: %v", e)
	}
	require.NoError(t, h.close())
}

func TestStartPairingFailureClearsSlot(t *testing.T) {
	f := newFake()
	f.err["login"] = fmt.Errorf("relay down")
	h := start(t, f.factory())
	h.send(req(1, &pb.StartPairing{}))
	require.Equal(t, pb.ErrorCode_ERROR_CODE_INTERNAL, h.response(1).GetError().GetCode())
	h.send(req(2, &pb.RefreshPairing{}))
	require.Equal(t, pb.ErrorCode_ERROR_CODE_NOT_CONNECTED, h.response(2).GetError().GetCode())
	require.NoError(t, h.close())
}

func TestConnectPassesSessionAndForwardsEvents(t *testing.T) {
	h, f := connected(t)
	require.Equal(t, []byte("sess"), f.sessionArg())
	require.Equal(t, []string{"new", "config", "connect"}, f.Calls())

	f.fire(&events.PhoneNotResponding{})
	st := h.event(func(e *pb.Event) bool { return e.GetConnectionState() != nil })
	require.Equal(t, pb.ConnState_CONN_STATE_PHONE_OFFLINE, st.GetConnectionState().GetState())

	f.fire(&events.AuthTokenRefreshed{})
	up := h.event(func(e *pb.Event) bool { return e.GetSessionUpdated() != nil })
	require.Equal(t, []byte(`{"fake":true}`), up.GetSessionUpdated().GetSessionBlob())
	require.NoError(t, h.close())
}

func TestConnectCredentialErrorIsUnauthorized(t *testing.T) {
	f := newFake()
	f.err["connect"] = fmt.Errorf("refresh: %w", events.ErrInvalidCredentials)
	h := start(t, f.factory())
	h.send(req(1, &pb.Connect{SessionBlob: []byte("s")}))
	require.Equal(t, pb.ErrorCode_ERROR_CODE_UNAUTHORIZED, h.response(1).GetError().GetCode())
	require.NoError(t, h.close())
}

func TestConnectRequiresBlob(t *testing.T) {
	h := start(t, newFake().factory())
	h.send(req(1, &pb.Connect{}))
	require.Equal(t, pb.ErrorCode_ERROR_CODE_BAD_REQUEST, h.response(1).GetError().GetCode())
	require.NoError(t, h.close())
}

func TestDisconnectAndUnpair(t *testing.T) {
	h, f := connected(t)
	h.send(req(2, &pb.SetActive{}))
	require.NotNil(t, h.response(2).GetAck())
	h.send(req(3, &pb.Unpair{}))
	require.NotNil(t, h.response(3).GetAck())
	require.Contains(t, f.Calls(), "unpair")
	h.send(req(4, &pb.SetActive{}))
	require.Equal(t, pb.ErrorCode_ERROR_CODE_NOT_CONNECTED, h.response(4).GetError().GetCode())

	h.send(req(5, &pb.Connect{SessionBlob: []byte("s2")}))
	require.NotNil(t, h.response(5).GetAck())
	h.send(req(6, &pb.Disconnect{}))
	require.NotNil(t, h.response(6).GetAck())
	h.send(req(7, &pb.SetActive{}))
	require.Equal(t, pb.ErrorCode_ERROR_CODE_NOT_CONNECTED, h.response(7).GetError().GetCode())
	require.NoError(t, h.close())
}

func TestStaleClientEventsDropped(t *testing.T) {
	old := newFake()
	cur := newFake()
	n := 0
	h := start(t, func(s []byte) (gmClient, error) {
		n++
		if n == 1 {
			return old.factory()(s)
		}
		return cur.factory()(s)
	})
	h.send(req(1, &pb.Connect{SessionBlob: []byte("a")}))
	require.NotNil(t, h.response(1).GetAck())
	h.send(req(2, &pb.Connect{SessionBlob: []byte("b")}))
	require.NotNil(t, h.response(2).GetAck())
	require.Contains(t, old.Calls(), "disconnect")

	old.fire(&events.PhoneNotResponding{}) // stale: dropped
	cur.fire(&events.PhoneRespondingAgain{})
	st := h.event(func(e *pb.Event) bool { return e.GetConnectionState() != nil })
	require.Equal(t, pb.ConnState_CONN_STATE_PHONE_ONLINE, st.GetConnectionState().GetState())
	require.NoError(t, h.close())
}
