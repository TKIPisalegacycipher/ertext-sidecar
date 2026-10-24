package server

import (
	"bytes"
	"context"
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

	connectGate chan struct{} // if set, Connect blocks until it is closed

	listReq   *gmproto.ListConversationsRequest
	listResp  *gmproto.ListConversationsResponse
	fetchArgs []any
	fetchResp *gmproto.ListMessagesResponse
	sendReq   *gmproto.SendMessageRequest
	sendResp  *gmproto.SendMessageResponse
	reactReq  *gmproto.SendReactionRequest
	reactResp *gmproto.SendReactionResponse
	readArgs  [2]string
	media     []byte
	thumbIDs  []string
	thumbs    *gmproto.GetThumbnailResponse
}

func newFake() *fakeClient {
	return &fakeClient{blob: []byte(`{"fake":true}`), qr: "https://qr/1", refreshQR: "https://qr/2", err: map[string]error{}}
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
func (f *fakeClient) Disconnect()                            { _ = f.record("disconnect") }
func (f *fakeClient) Unpair(context.Context) error           { return f.record("unpair") }
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
