package server

import (
	"context"
	"fmt"

	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/gm"
	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/pb"
	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/translate"
)

func (s *Server) handle(req *pb.Request) *pb.Response {
	switch b := req.GetBody().(type) {
	case *pb.Request_Ping:
		return &pb.Response{Result: &pb.Response_Pong{Pong: &pb.Pong{}}}
	case *pb.Request_Connect:
		return s.connect(b.Connect)
	case *pb.Request_SetActive:
		return s.withClient(requestTimeout, func(ctx context.Context, c gm.Client) *pb.Response {
			return ackOr(c.SetActiveSession(ctx))
		})
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

// handlerFor forwards c's libgm events while c is the current client.
func (s *Server) handlerFor(c gm.Client) func(evt any) {
	return func(evt any) {
		if s.current() != c {
			return
		}
		for _, e := range translate.Event(evt) {
			s.emit(e)
		}
	}
}
