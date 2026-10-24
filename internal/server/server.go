// Package server runs the sidecar protocol: it reads Request frames, answers each with one
// Response, and forwards libgm events as Event frames. Its only state is the current libgm client.
package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"google.golang.org/protobuf/proto"

	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/frame"
	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/gm"
	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/pb"
)

const ProtocolVersion = 1

const (
	requestTimeout  = 2 * time.Minute
	downloadTimeout = 10 * time.Minute
)

type Server struct {
	newClient      gm.Factory
	log            zerolog.Logger
	sidecarVersion string
	libgmVersion   string

	// PairSettle is how long to wait after pairing before reporting success, so the phone saves
	// the pairing first (mautrix-gmessages waits 2 s for the same reason).
	PairSettle time.Duration

	ctx context.Context
	out chan *pb.Envelope

	mu  sync.Mutex
	cli gm.Client
}

func New(f gm.Factory, log zerolog.Logger, sidecarVersion, libgmVersion string) *Server {
	return &Server{newClient: f, log: log, sidecarVersion: sidecarVersion, libgmVersion: libgmVersion, PairSettle: 2 * time.Second}
}

// Run serves until r reaches EOF (nil), a frame can't be decoded, or w fails (error).
func (s *Server) Run(ctx context.Context, r io.Reader, w io.Writer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	s.ctx = ctx
	s.out = make(chan *pb.Envelope, 256)

	writeErr := make(chan error, 1)
	go func() { writeErr <- s.writeLoop(ctx, cancel, w) }()

	s.emit(&pb.Event{Body: &pb.Event_Hello{Hello: &pb.Hello{
		ProtocolVersion: ProtocolVersion, SidecarVersion: s.sidecarVersion, LibgmVersion: s.libgmVersion,
	}}})

	var handlers sync.WaitGroup
	readErr := s.readLoop(r, &handlers)

	s.replace(nil) // disconnect libgm before exiting
	cancel()
	handlers.Wait()
	if err := <-writeErr; err != nil {
		return err
	}
	return readErr
}

func (s *Server) readLoop(r io.Reader, handlers *sync.WaitGroup) error {
	for {
		buf, err := frame.Read(r)
		if errors.Is(err, io.EOF) {
			return nil
		} else if err != nil {
			return fmt.Errorf("read frame: %w", err)
		}
		var env pb.Envelope
		if err := proto.Unmarshal(buf, &env); err != nil {
			return fmt.Errorf("decode frame: %w", err)
		}
		req := env.GetRequest()
		if req == nil {
			s.log.Warn().Msg("ignoring non-request frame")
			continue
		}
		handlers.Add(1)
		go func() {
			defer handlers.Done()
			res := s.handle(req)
			res.Id = req.GetId()
			s.send(&pb.Envelope{Kind: &pb.Envelope_Response{Response: res}})
		}()
	}
}

func (s *Server) writeLoop(ctx context.Context, cancel context.CancelFunc, w io.Writer) error {
	for {
		select {
		case env := <-s.out:
			b, err := proto.Marshal(env)
			if err == nil {
				err = frame.Write(w, b)
			}
			if err != nil {
				cancel()
				return fmt.Errorf("write frame: %w", err)
			}
		case <-ctx.Done():
			return nil
		}
	}
}

func (s *Server) send(env *pb.Envelope) {
	select {
	case s.out <- env:
	case <-s.ctx.Done():
	}
}

func (s *Server) emit(e *pb.Event) { s.send(&pb.Envelope{Kind: &pb.Envelope_Event{Event: e}}) }

func (s *Server) current() gm.Client {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cli
}

// replace makes c the current client and disconnects the previous one.
func (s *Server) replace(c gm.Client) {
	s.mu.Lock()
	old := s.cli
	s.cli = c
	s.mu.Unlock()
	if old != nil && old != c {
		old.Disconnect()
	}
}

// dropIf clears the slot if c is still current; false means c was superseded.
func (s *Server) dropIf(c gm.Client) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cli != c {
		return false
	}
	s.cli = nil
	return true
}

// withClient runs fn against the current client under a request timeout.
func (s *Server) withClient(timeout time.Duration, fn func(context.Context, gm.Client) *pb.Response) *pb.Response {
	c := s.current()
	if c == nil {
		return errResp(errNotConnected)
	}
	ctx, cancel := context.WithTimeout(s.ctx, timeout)
	defer cancel()
	return fn(ctx, c)
}
