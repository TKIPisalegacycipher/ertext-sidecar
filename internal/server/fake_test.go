package server

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"go.mau.fi/mautrix-gmessages/pkg/libgm"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"
	"google.golang.org/protobuf/proto"

	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/frame"
	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/gm"
	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/gmext"
	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/pb"
)

// fakeClient records calls and returns canned results. One instance per test; the factory hands
// it out for every session (tests that need two clients build two fakes).
type fakeClient struct {
	mu        sync.Mutex
	calls     []string
	handler   libgm.EventHandler
	pairCB    func(*gmproto.PairedData)
	session   []byte
	blob      []byte
	qr        string
	refreshQR string
	err       map[string]error

	cookies   map[string]string
	gaiaEmoji []string      // DoGaiaPairing reports each of these to its callback
	gaiaGate  chan struct{} // if set, DoGaiaPairing blocks (after the emoji) until closed

	connectGate chan struct{}   // if set, Connect blocks until it is closed
	unpairGate  chan struct{}   // if set, Unpair blocks until it is closed
	panicOn     map[string]bool // record(name) panics for these calls

	listReq    *gmproto.ListConversationsRequest
	listResp   *gmproto.ListConversationsResponse
	fetchArgs  []any
	fetchResp  *gmproto.ListMessagesResponse
	sendReq    *gmproto.SendMessageRequest
	sendResp   *gmproto.SendMessageResponse
	reactReq   *gmproto.SendReactionRequest
	reactResp  *gmproto.SendReactionResponse
	readArgs   [2]string
	updateReq  *gmproto.UpdateConversationRequest
	updateResp *gmproto.UpdateConversationResponse
	deleteArgs [2]string
	fullArgs   [2]string
	media      []byte
	thumbIDs   []string
	thumbs     *gmproto.GetThumbnailResponse

	createReqs  []*gmproto.GetOrCreateConversationRequest
	createResps []*gmproto.GetOrCreateConversationResponse // answered in order; the last one repeats
	contacts    *gmproto.ListContactsResponse
	topContacts *gmproto.ListTopContactsResponse

	rcsReqs []*gmext.GetContactRcsGroupStatusRequest
	rcs     map[string]*gmext.GetContactRcsGroupStatusResponse // by number; missing: an error
}

func newFake() *fakeClient {
	return &fakeClient{blob: []byte(`{"fake":true}`), qr: "https://qr/1", refreshQR: "https://qr/2",
		err: map[string]error{}, panicOn: map[string]bool{},
		updateResp: &gmproto.UpdateConversationResponse{Success: true}}
}

func (f *fakeClient) factory() gm.Factory {
	return func(session []byte) (gm.Client, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.session = session
		f.calls = append(f.calls, "new")
		return f, f.err["new"]
	}
}

func (f *fakeClient) record(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, name)
	if f.panicOn[name] {
		panic("fake panic: " + name)
	}
	return f.err[name]
}

func (f *fakeClient) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

// fire delivers a libgm event as libgm would (from its own goroutine).
func (f *fakeClient) fire(evt any) {
	f.mu.Lock()
	h := f.handler
	f.mu.Unlock()
	h(evt)
}

func (f *fakeClient) SetEventHandler(h libgm.EventHandler) {
	f.mu.Lock()
	f.handler = h
	f.mu.Unlock()
}
func (f *fakeClient) SetPairCallback(cb func(*gmproto.PairedData)) {
	f.mu.Lock()
	f.pairCB = cb
	f.mu.Unlock()
}
func (f *fakeClient) SessionBlob() ([]byte, error) {
	f.mu.Lock()
	b := f.blob
	f.mu.Unlock()
	return b, f.record("blob")
}
func (f *fakeClient) FetchConfig(context.Context) error { return f.record("config") }
func (f *fakeClient) SetCookies(c map[string]string) {
	f.mu.Lock()
	f.cookies = c
	f.mu.Unlock()
	_ = f.record("cookies")
}
func (f *fakeClient) DoGaiaPairing(ctx context.Context, emoji func(string)) error {
	err := f.record("gaia")
	f.mu.Lock()
	em, gate := f.gaiaEmoji, f.gaiaGate
	f.mu.Unlock()
	for _, e := range em {
		emoji(e)
	}
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return err
}
func (f *fakeClient) gotCookies() map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cookies
}
func (f *fakeClient) StartLogin(context.Context) (string, error) {
	f.mu.Lock()
	q := f.qr
	f.mu.Unlock()
	return q, f.record("login")
}
func (f *fakeClient) RefreshPhoneRelay() (string, error) {
	f.mu.Lock()
	q := f.refreshQR
	f.mu.Unlock()
	return q, f.record("refresh")
}
func (f *fakeClient) Connect(context.Context) error {
	err := f.record("connect")
	f.mu.Lock()
	gate := f.connectGate
	f.mu.Unlock()
	if gate != nil {
		<-gate
	}
	return err
}
func (f *fakeClient) Disconnect() { _ = f.record("disconnect") }
func (f *fakeClient) Unpair(context.Context) error {
	err := f.record("unpair")
	f.mu.Lock()
	gate := f.unpairGate
	f.mu.Unlock()
	if gate != nil {
		<-gate
	}
	return err
}
func (f *fakeClient) SetActiveSession(context.Context) error { return f.record("setactive") }
func (f *fakeClient) ListConversations(_ context.Context, r *gmproto.ListConversationsRequest) (*gmproto.ListConversationsResponse, error) {
	f.mu.Lock()
	f.listReq = r
	resp := f.listResp
	f.mu.Unlock()
	return resp, f.record("list")
}
func (f *fakeClient) FetchMessages(_ context.Context, id string, n int64, c *gmproto.Cursor) (*gmproto.ListMessagesResponse, error) {
	f.mu.Lock()
	f.fetchArgs = []any{id, n, c}
	resp := f.fetchResp
	f.mu.Unlock()
	return resp, f.record("fetch")
}
func (f *fakeClient) SendMessage(_ context.Context, r *gmproto.SendMessageRequest) (*gmproto.SendMessageResponse, error) {
	f.mu.Lock()
	f.sendReq = r
	resp := f.sendResp
	f.mu.Unlock()
	return resp, f.record("send")
}
func (f *fakeClient) SendReaction(_ context.Context, r *gmproto.SendReactionRequest) (*gmproto.SendReactionResponse, error) {
	f.mu.Lock()
	f.reactReq = r
	resp := f.reactResp
	f.mu.Unlock()
	return resp, f.record("react")
}
func (f *fakeClient) MarkRead(_ context.Context, c, m string) error {
	f.mu.Lock()
	f.readArgs = [2]string{c, m}
	f.mu.Unlock()
	return f.record("markread")
}
func (f *fakeClient) GetOrCreateConversation(_ context.Context, r *gmproto.GetOrCreateConversationRequest) (*gmproto.GetOrCreateConversationResponse, error) {
	f.mu.Lock()
	// A copy: the handler changes the request before asking again.
	f.createReqs = append(f.createReqs, proto.Clone(r).(*gmproto.GetOrCreateConversationRequest))
	var resp *gmproto.GetOrCreateConversationResponse
	if n := len(f.createResps); n > 0 {
		resp = f.createResps[0]
		if n > 1 {
			f.createResps = f.createResps[1:]
		}
	}
	f.mu.Unlock()
	return resp, f.record("create")
}
func (f *fakeClient) ListContacts(context.Context) (*gmproto.ListContactsResponse, error) {
	f.mu.Lock()
	resp := f.contacts
	f.mu.Unlock()
	return resp, f.record("contacts")
}
func (f *fakeClient) ListTopContacts(context.Context) (*gmproto.ListTopContactsResponse, error) {
	f.mu.Lock()
	resp := f.topContacts
	f.mu.Unlock()
	return resp, f.record("topcontacts")
}
func (f *fakeClient) GetContactRCSGroupStatus(_ context.Context, r *gmext.GetContactRcsGroupStatusRequest) (*gmext.GetContactRcsGroupStatusResponse, error) {
	f.mu.Lock()
	f.rcsReqs = append(f.rcsReqs, r)
	resp, ok := f.rcs[r.GetNumber().GetNumber()]
	f.mu.Unlock()
	if err := f.record("rcs"); err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("no answer")
	}
	return resp, nil
}
func (f *fakeClient) setRCS(m map[string]*gmext.GetContactRcsGroupStatusResponse) {
	f.mu.Lock()
	f.rcs = m
	f.mu.Unlock()
}
func (f *fakeClient) gotRCS() []*gmext.GetContactRcsGroupStatusRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*gmext.GetContactRcsGroupStatusRequest(nil), f.rcsReqs...)
}
func (f *fakeClient) setCreate(rs ...*gmproto.GetOrCreateConversationResponse) {
	f.mu.Lock()
	f.createResps = rs
	f.mu.Unlock()
}
func (f *fakeClient) gotCreate() []*gmproto.GetOrCreateConversationRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*gmproto.GetOrCreateConversationRequest(nil), f.createReqs...)
}
func (f *fakeClient) setContacts(all *gmproto.ListContactsResponse, top *gmproto.ListTopContactsResponse) {
	f.mu.Lock()
	f.contacts, f.topContacts = all, top
	f.mu.Unlock()
}
func (f *fakeClient) UpdateConversation(_ context.Context, r *gmproto.UpdateConversationRequest) (*gmproto.UpdateConversationResponse, error) {
	f.mu.Lock()
	f.updateReq = r
	resp := f.updateResp
	f.mu.Unlock()
	return resp, f.record("update")
}
func (f *fakeClient) DeleteConversation(_ context.Context, c, phone string) error {
	f.mu.Lock()
	f.deleteArgs = [2]string{c, phone}
	f.mu.Unlock()
	return f.record("delete")
}
func (f *fakeClient) setUpdate(r *gmproto.UpdateConversationResponse) {
	f.mu.Lock()
	f.updateResp = r
	f.mu.Unlock()
}
func (f *fakeClient) gotUpdate() *gmproto.UpdateConversationRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.updateReq
}
func (f *fakeClient) gotDelete() [2]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.deleteArgs
}
func (f *fakeClient) GetFullSizeImage(_ context.Context, m, a string) (*gmproto.GetFullSizeImageResponse, error) {
	f.mu.Lock()
	f.fullArgs = [2]string{m, a}
	f.mu.Unlock()
	return &gmproto.GetFullSizeImageResponse{}, f.record("fullsize")
}
func (f *fakeClient) DownloadMedia(context.Context, string, []byte) (io.ReadCloser, error) {
	f.mu.Lock()
	media := f.media
	f.mu.Unlock()
	return io.NopCloser(bytes.NewReader(media)), f.record("download")
}
func (f *fakeClient) GetParticipantThumbnail(_ context.Context, ids ...string) (*gmproto.GetThumbnailResponse, error) {
	f.mu.Lock()
	f.thumbIDs = ids
	resp := f.thumbs
	f.mu.Unlock()
	return resp, f.record("thumbs")
}

type harness struct {
	t      *testing.T
	in     *io.PipeWriter
	envs   chan *pb.Envelope
	done   chan error
	events []*pb.Event
}

func start(t *testing.T, f gm.Factory) *harness {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	srv := New(f, zerolog.Nop(), "test", "libgm-test")
	srv.PairSettle = 0
	h := &harness{t: t, in: inW, envs: make(chan *pb.Envelope, 64), done: make(chan error, 1)}
	go func() {
		err := srv.Run(context.Background(), inR, outW)
		_ = outW.Close()
		h.done <- err
	}()
	go func() {
		for {
			b, err := frame.Read(outR)
			if err != nil {
				close(h.envs)
				return
			}
			var env pb.Envelope
			if err := proto.Unmarshal(b, &env); err != nil {
				panic(err)
			}
			h.envs <- &env
		}
	}()
	t.Cleanup(func() { _ = inW.Close() })
	return h
}

func (h *harness) send(req *pb.Request) {
	h.t.Helper()
	b, err := proto.Marshal(&pb.Envelope{Kind: &pb.Envelope_Request{Request: req}})
	require.NoError(h.t, err)
	require.NoError(h.t, frame.Write(h.in, b))
}

func (h *harness) next() *pb.Envelope {
	h.t.Helper()
	select {
	case env, ok := <-h.envs:
		require.True(h.t, ok, "server output closed")
		return env
	case <-time.After(5 * time.Second):
		h.t.Fatal("timed out waiting for a frame")
		return nil
	}
}

// response reads frames until the response with id arrives; events seen on the way are kept.
func (h *harness) response(id uint64) *pb.Response {
	h.t.Helper()
	for {
		env := h.next()
		if r := env.GetResponse(); r != nil && r.GetId() == id {
			return r
		}
		if e := env.GetEvent(); e != nil {
			h.events = append(h.events, e)
		}
	}
}

// event returns the first already-seen or next event matching want.
func (h *harness) event(want func(*pb.Event) bool) *pb.Event {
	h.t.Helper()
	for i, e := range h.events {
		if want(e) {
			h.events = append(h.events[:i], h.events[i+1:]...)
			return e
		}
	}
	for {
		env := h.next()
		if e := env.GetEvent(); e != nil {
			if want(e) {
				return e
			}
			h.events = append(h.events, e)
		}
	}
}

func (h *harness) close() error {
	h.t.Helper()
	_ = h.in.Close()
	select {
	case err := <-h.done:
		return err
	case <-time.After(5 * time.Second):
		h.t.Fatal("server did not exit")
		return nil
	}
}

func isHello(e *pb.Event) bool { return e.GetHello() != nil }

func (f *fakeClient) sessionArg() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.session
}

type gmClient = gm.Client

// Locked accessors/setters for the data-request fields.
func (f *fakeClient) setList(r *gmproto.ListConversationsResponse) {
	f.mu.Lock()
	f.listResp = r
	f.mu.Unlock()
}
func (f *fakeClient) setFetch(r *gmproto.ListMessagesResponse) {
	f.mu.Lock()
	f.fetchResp = r
	f.mu.Unlock()
}
func (f *fakeClient) setSend(r *gmproto.SendMessageResponse) {
	f.mu.Lock()
	f.sendResp = r
	f.mu.Unlock()
}
func (f *fakeClient) setReact(r *gmproto.SendReactionResponse) {
	f.mu.Lock()
	f.reactResp = r
	f.mu.Unlock()
}
func (f *fakeClient) setMedia(b []byte) {
	f.mu.Lock()
	f.media = b
	f.mu.Unlock()
}
func (f *fakeClient) setThumbs(r *gmproto.GetThumbnailResponse) {
	f.mu.Lock()
	f.thumbs = r
	f.mu.Unlock()
}
func (f *fakeClient) gotList() *gmproto.ListConversationsRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.listReq
}
func (f *fakeClient) gotFetch() []any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]any(nil), f.fetchArgs...)
}
func (f *fakeClient) gotSend() *gmproto.SendMessageRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sendReq
}
func (f *fakeClient) gotReact() *gmproto.SendReactionRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reactReq
}
func (f *fakeClient) gotRead() [2]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.readArgs
}
func (f *fakeClient) gotThumbIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.thumbIDs...)
}
