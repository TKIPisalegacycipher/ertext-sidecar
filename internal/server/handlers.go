package server

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mau.fi/mautrix-gmessages/pkg/libgm/events"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"

	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/gm"
	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/pb"
	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/translate"
)

func (s *Server) handle(req *pb.Request) *pb.Response {
	switch b := req.GetBody().(type) {
	case *pb.Request_Ping:
		return &pb.Response{Result: &pb.Response_Pong{Pong: &pb.Pong{}}}
	case *pb.Request_StartPairing:
		return s.startPairing()
	case *pb.Request_RefreshPairing:
		return s.refreshPairing()
	case *pb.Request_Connect:
		return s.connect(b.Connect)
	case *pb.Request_Disconnect:
		s.replace(nil)
		return ack()
	case *pb.Request_Unpair:
		return s.unpair()
	case *pb.Request_SetActive:
		return s.withClient(requestTimeout, func(ctx context.Context, c gm.Client) *pb.Response {
			return ackOr(c.SetActiveSession(ctx))
		})
	case *pb.Request_ListConversations:
		return s.listConversations(b.ListConversations)
	case *pb.Request_FetchMessages:
		return s.fetchMessages(b.FetchMessages)
	case *pb.Request_SendMessage:
		return s.sendMessage(b.SendMessage)
	case *pb.Request_SendReaction:
		return s.sendReaction(b.SendReaction)
	case *pb.Request_MarkRead:
		return s.markRead(b.MarkRead)
	case *pb.Request_DownloadMedia:
		return s.downloadMedia(b.DownloadMedia)
	case *pb.Request_GetThumbnails:
		return s.getThumbnails(b.GetThumbnails)
	default:
		return errResp(fmt.Errorf("%w: unknown request body %T", translate.ErrBadRequest, b))
	}
}

func (s *Server) connect(r *pb.Connect) *pb.Response {
	if len(r.GetSessionBlob()) == 0 {
		return errResp(fmt.Errorf("%w: session_blob required", translate.ErrBadRequest))
	}
	c, err := s.newClient(r.GetSessionBlob())
	if err != nil {
		return errResp(err)
	}
	c.SetEventHandler(s.handlerFor(c))
	s.replace(c)
	ctx, cancel := context.WithTimeout(s.ctx, requestTimeout)
	defer cancel()
	if err := c.FetchConfig(ctx); err != nil {
		s.log.Warn().Err(err).Msg("fetch config failed; connecting anyway") // as mautrix-gmessages does
	}
	if err := c.Connect(s.ctx); err != nil { // long polling lives on the server context
		s.dropIf(c)
		return errResp(err)
	}
	if s.current() != c { // a later Connect/StartPairing replaced c while it was connecting
		c.Disconnect()
		return errResp(fmt.Errorf("connect superseded by a later Connect/StartPairing"))
	}
	return ack()
}

// handlerFor forwards c's libgm events while c is the current client. AuthTokenRefreshed becomes
// SessionUpdated, so Rust always holds the latest blob.
func (s *Server) handlerFor(c gm.Client) func(evt any) {
	return func(evt any) {
		if s.current() != c {
			return
		}
		if _, ok := evt.(*events.AuthTokenRefreshed); ok {
			blob, err := c.SessionBlob()
			if err != nil {
				s.log.Error().Err(err).Msg("marshal refreshed session")
				return
			}
			s.emit(&pb.Event{Body: &pb.Event_SessionUpdated{SessionUpdated: &pb.SessionUpdated{SessionBlob: blob}}})
			return
		}
		for _, e := range translate.Event(evt) {
			s.emit(e)
		}
	}
}

func pairQr(url string) *pb.Event {
	return &pb.Event{Body: &pb.Event_PairQr{PairQr: &pb.PairQr{Url: url}}}
}

func (s *Server) startPairing() *pb.Response {
	c, err := s.newClient(nil)
	if err != nil {
		return errResp(err)
	}
	c.SetEventHandler(func(evt any) { s.log.Debug().Type("event", evt).Msg("ignoring pre-pairing event") })
	c.SetPairCallback(func(*gmproto.PairedData) { go s.pairDone(c) })
	s.replace(c)
	ctx, cancel := context.WithTimeout(s.ctx, requestTimeout)
	defer cancel()
	if err := c.FetchConfig(ctx); err != nil {
		s.dropIf(c)
		return errResp(err)
	}
	url, err := c.StartLogin(s.ctx) // its long poll lives on the server context
	if err != nil {
		s.dropIf(c)
		return errResp(err)
	}
	if s.current() != c { // a later Connect/StartPairing replaced c while it was starting
		c.Disconnect()
		return errResp(fmt.Errorf("pairing superseded by a later Connect/StartPairing"))
	}
	s.emit(pairQr(url))
	return ack()
}

func (s *Server) refreshPairing() *pb.Response {
	c := s.current()
	if c == nil {
		return errResp(errNotConnected)
	}
	url, err := c.RefreshPhoneRelay()
	if err != nil {
		return errResp(err)
	}
	s.emit(pairQr(url))
	return ack()
}

// pairDone mirrors mautrix-gmessages' finishLogin: take the session, drop the pairing client,
// give the phone a moment to save the pairing, then hand the session to Rust (which Connects).
func (s *Server) pairDone(c gm.Client) {
	blob, err := c.SessionBlob()
	c.Disconnect()
	if !s.dropIf(c) {
		return // superseded by another StartPairing/Connect
	}
	if err != nil {
		s.emit(&pb.Event{Body: &pb.Event_ConnectionState{ConnectionState: &pb.ConnectionState{
			State: pb.ConnState_CONN_STATE_ERROR, Detail: "pairing succeeded but session could not be saved",
		}}})
		return
	}
	select {
	case <-time.After(s.PairSettle):
	case <-s.ctx.Done():
		return
	}
	s.emit(&pb.Event{Body: &pb.Event_PairSuccess{PairSuccess: &pb.PairSuccess{SessionBlob: blob}}})
}

func (s *Server) unpair() *pb.Response {
	c := s.current()
	if c == nil {
		return errResp(errNotConnected)
	}
	ctx, cancel := context.WithTimeout(s.ctx, requestTimeout)
	defer cancel()
	err := c.Unpair(ctx)
	s.replace(nil)
	return ackOr(err)
}

func (s *Server) listConversations(*pb.ListConversations) *pb.Response {
	return errResp(errNotImplemented)
}
func (s *Server) fetchMessages(*pb.FetchMessages) *pb.Response { return errResp(errNotImplemented) }
func (s *Server) sendMessage(*pb.SendMessage) *pb.Response     { return errResp(errNotImplemented) }
func (s *Server) sendReaction(*pb.SendReaction) *pb.Response   { return errResp(errNotImplemented) }
func (s *Server) markRead(*pb.MarkRead) *pb.Response           { return errResp(errNotImplemented) }
func (s *Server) downloadMedia(*pb.DownloadMedia) *pb.Response { return errResp(errNotImplemented) }
func (s *Server) getThumbnails(*pb.GetThumbnails) *pb.Response { return errResp(errNotImplemented) }

var errNotImplemented = errors.New("not implemented")
