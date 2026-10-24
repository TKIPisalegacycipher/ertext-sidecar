package server

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"go.mau.fi/mautrix-gmessages/pkg/libgm"
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
	case *pb.Request_StartGoogleLogin:
		return s.startGoogleLogin(b.StartGoogleLogin)
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
	case *pb.Request_RequestFullMedia:
		return s.requestFullMedia(b.RequestFullMedia)
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
		s.abandon(c)
		return errResp(authRejected(err))
	}
	if s.current() != c { // a later Connect/StartPairing replaced c while it was connecting
		c.Disconnect()
		return errResp(fmt.Errorf("connect superseded by a later Connect/StartPairing"))
	}
	return ack()
}

// abandon drops c after a failed start. libgm may already be long polling (StartLogin starts the
// poll before it can fail), so c is disconnected whether or not it is still current. Disconnect is
// idempotent, so disconnecting a client a later request already replaced is harmless.
func (s *Server) abandon(c gm.Client) {
	s.dropIf(c)
	c.Disconnect()
}

// handlerFor forwards c's libgm events while c is the current client. AuthTokenRefreshed becomes
// SessionUpdated, so Rust always holds the latest blob. Like mautrix-gmessages, MESSAGE_DELETED is
// ignored while the phone's database syncs (the flag is per client); when the sync completes,
// translate emits ResyncNeeded. A panic is logged (value type only) and the event dropped.
func (s *Server) handlerFor(c gm.Client) func(evt any) {
	var dbSyncing atomic.Bool
	return func(evt any) {
		defer func() {
			if p := recover(); p != nil {
				s.log.Error().Str("panic_type", fmt.Sprintf("%T", p)).Str("event", fmt.Sprintf("%T", evt)).
					Msg("event handler panicked; event dropped")
			}
		}()
		if s.current() != c {
			return
		}
		if syncing, ok := translate.MobileDBSync(evt); ok {
			dbSyncing.Store(syncing)
		} else if dbSyncing.Load() && translate.SuppressedDuringDBSync(evt) {
			s.log.Debug().Msg("ignoring MESSAGE_DELETED during mobile database sync")
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
		if wm, ok := evt.(*libgm.WrappedMessage); ok && wm != nil && wm.Message != nil {
			s.logMedia(wm.Message)
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
		s.abandon(c)
		return errResp(err)
	}
	url, err := c.StartLogin(s.ctx) // its long poll lives on the server context
	if err != nil {
		s.abandon(c) // StartLogin may have started the long poll before failing
		return errResp(err)
	}
	if s.current() != c { // a later Connect/StartPairing replaced c while it was starting
		c.Disconnect()
		return errResp(fmt.Errorf("pairing superseded by a later Connect/StartPairing"))
	}
	s.emit(pairQr(url))
	return ack()
}

// googleLoginTimeout bounds the whole Google-account pairing, including the wait for the user to
// tap the emoji on the phone.
const googleLoginTimeout = 5 * time.Minute

// startGoogleLogin runs Google-account ("gaia") pairing and replies only once it has finished.
// Like pairDone, on success it takes the session, drops the pairing client, waits PairSettle and
// emits PairSuccess (Rust then Connects); the Ack follows. Cookies, the emoji and the session are
// never logged.
func (s *Server) startGoogleLogin(r *pb.StartGoogleLogin) *pb.Response {
	if len(r.GetCookies()) == 0 {
		return errResp(badRequest("cookies required"))
	}
	c, err := s.newClient(nil)
	if err != nil {
		return errResp(err)
	}
	c.SetCookies(r.GetCookies())
	c.SetEventHandler(func(evt any) { s.log.Debug().Type("event", evt).Msg("ignoring pre-pairing event") })
	s.replace(c)
	ctx, cancel := context.WithTimeout(s.ctx, googleLoginTimeout)
	defer cancel()
	fail := func(err error) *pb.Response {
		superseded := s.current() != c
		s.abandon(c) // pairing may have started a long poll before failing
		if superseded {
			return errResp(fmt.Errorf("google login %w", errSuperseded))
		}
		return errResp(signInRejected(err))
	}
	if err := c.FetchConfig(ctx); err != nil {
		return fail(err)
	}
	err = c.DoGaiaPairing(ctx, func(emoji string) {
		if s.current() == c {
			s.emit(&pb.Event{Body: &pb.Event_PairEmoji{PairEmoji: &pb.PairEmoji{Emoji: emoji}}})
		}
	})
	if err != nil {
		return fail(err)
	}
	blob, err := c.SessionBlob()
	c.Disconnect()
	if !s.dropIf(c) {
		return errResp(fmt.Errorf("google login %w", errSuperseded))
	}
	if err != nil {
		return errResp(fmt.Errorf("pairing succeeded but session could not be saved: %w", err))
	}
	select {
	case <-time.After(s.PairSettle):
	case <-s.ctx.Done():
		return errResp(s.ctx.Err())
	}
	s.emit(&pb.Event{Body: &pb.Event_PairSuccess{PairSuccess: &pb.PairSuccess{SessionBlob: blob}}})
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
	if s.dropIf(c) { // a Connect/StartPairing during Unpair keeps its newer client
		c.Disconnect()
	}
	return ackOr(err)
}

func badRequest(format string, args ...any) error {
	return fmt.Errorf("%w: %s", translate.ErrBadRequest, fmt.Sprintf(format, args...))
}

func (s *Server) listConversations(r *pb.ListConversations) *pb.Response {
	count := int64(r.GetCount())
	if count <= 0 {
		count = 100
	}
	return s.withClient(requestTimeout, func(ctx context.Context, c gm.Client) *pb.Response {
		resp, err := c.ListConversations(ctx, &gmproto.ListConversationsRequest{
			Count: count, Folder: translate.Folder(r.GetFolder()), Cursor: translate.GMCursor(r.GetCursor()),
		})
		if err != nil {
			return errResp(err)
		}
		return &pb.Response{Result: &pb.Response_ConversationPage{ConversationPage: translate.ConversationPage(resp)}}
	})
}

func (s *Server) fetchMessages(r *pb.FetchMessages) *pb.Response {
	if r.GetConversationId() == "" {
		return errResp(badRequest("conversation_id required"))
	}
	count := int64(r.GetCount())
	if count <= 0 {
		count = 25
	}
	cur := translate.GMCursor(r.GetCursor())
	return s.withClient(requestTimeout, func(ctx context.Context, c gm.Client) *pb.Response {
		resp, err := c.FetchMessages(ctx, r.GetConversationId(), count, cur)
		if err != nil {
			return errResp(err)
		}
		s.logMedia(resp.GetMessages()...)
		return &pb.Response{Result: &pb.Response_MessagePage{MessagePage: translate.MessagePage(resp, cur)}}
	})
}

// logMedia writes a debug line for each message that carries media or a location: how many
// MessageInfo entries of each type, how many media have no media_id, and the embedded byte count.
// Counts only, never content.
func (s *Server) logMedia(msgs ...*gmproto.Message) {
	if !s.log.Debug().Enabled() {
		return
	}
	for _, m := range msgs {
		st := translate.MediaStats(m)
		if st.Media+st.Location == 0 {
			continue
		}
		s.log.Debug().Str("message_id", m.GetMessageID()).
			Int("text", st.Text).Int("media", st.Media).Int("location", st.Location).Int("other", st.Other).
			Int("empty_media_id", st.EmptyMediaID).Int("embedded_bytes", st.EmbeddedBytes).
			Msg("message media")
	}
}

func (s *Server) sendMessage(r *pb.SendMessage) *pb.Response {
	gmReq, err := translate.SendMessageRequest(r)
	if err != nil {
		return errResp(err)
	}
	return s.withClient(requestTimeout, func(ctx context.Context, c gm.Client) *pb.Response {
		resp, err := c.SendMessage(ctx, gmReq)
		if err != nil {
			return errResp(err)
		}
		if resp.GetStatus() != gmproto.SendMessageResponse_SUCCESS {
			return errResp(fmt.Errorf("%w: %s", errSendFailed, resp.GetStatus()))
		}
		return ack()
	})
}

func (s *Server) sendReaction(r *pb.SendReaction) *pb.Response {
	gmReq, err := translate.SendReactionRequest(r)
	if err != nil {
		return errResp(err)
	}
	return s.withClient(requestTimeout, func(ctx context.Context, c gm.Client) *pb.Response {
		resp, err := c.SendReaction(ctx, gmReq)
		if err != nil {
			return errResp(err)
		}
		if !resp.GetSuccess() {
			return errResp(fmt.Errorf("%w: reaction rejected", errSendFailed))
		}
		return ack()
	})
}

func (s *Server) markRead(r *pb.MarkRead) *pb.Response {
	if r.GetConversationId() == "" || r.GetMessageId() == "" {
		return errResp(badRequest("conversation_id and message_id required"))
	}
	return s.withClient(requestTimeout, func(ctx context.Context, c gm.Client) *pb.Response {
		return ackOr(c.MarkRead(ctx, r.GetConversationId(), r.GetMessageId()))
	})
}

// requestFullMedia asks the phone to upload the original; it answers later with a message update
// whose media has a media_id, so this only acks that the request was made.
func (s *Server) requestFullMedia(r *pb.RequestFullMedia) *pb.Response {
	if r.GetMessageId() == "" || r.GetActionMessageId() == "" {
		return errResp(badRequest("message_id and action_message_id required"))
	}
	return s.withClient(requestTimeout, func(ctx context.Context, c gm.Client) *pb.Response {
		_, err := c.GetFullSizeImage(ctx, r.GetMessageId(), r.GetActionMessageId())
		s.log.Info().Str("message_id", r.GetMessageId()).Bool("ok", err == nil).Msg("requested full-size media")
		return ackOr(err)
	})
}

func (s *Server) getThumbnails(r *pb.GetThumbnails) *pb.Response {
	if len(r.GetParticipantIds()) == 0 {
		return errResp(badRequest("participant_ids required"))
	}
	return s.withClient(requestTimeout, func(ctx context.Context, c gm.Client) *pb.Response {
		resp, err := c.GetParticipantThumbnail(ctx, r.GetParticipantIds()...)
		if err != nil {
			return errResp(err)
		}
		out := &pb.Thumbnails{}
		for _, th := range resp.GetThumbnail() {
			out.Thumbnails = append(out.Thumbnails, &pb.Thumbnail{
				ParticipantId: th.GetIdentifier(), Image: th.GetData().GetImageBuffer(),
			})
		}
		return &pb.Response{Result: &pb.Response_Thumbnails{Thumbnails: out}}
	})
}

func (s *Server) downloadMedia(r *pb.DownloadMedia) *pb.Response {
	dest := r.GetDestPath()
	if r.GetMediaId() == "" || !filepath.IsAbs(dest) {
		return errResp(badRequest("media_id and an absolute dest_path required"))
	}
	return s.withClient(downloadTimeout, func(ctx context.Context, c gm.Client) *pb.Response {
		return ackOr(download(ctx, c, r.GetMediaId(), r.GetKey(), dest))
	})
}

// download decrypts via libgm into dest+".part", then renames over dest.
func download(ctx context.Context, c gm.Client, mediaID string, key []byte, dest string) (err error) {
	rc, err := c.DownloadMedia(ctx, mediaID, key)
	if err != nil {
		return err
	}
	defer rc.Close()
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return err
	}
	part := dest + ".part"
	f, err := os.Create(part)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = os.Remove(part)
		}
	}()
	if _, err = io.Copy(f, rc); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(part, dest)
}
