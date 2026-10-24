// Package gm is the only place that touches *libgm.Client. Everything else depends on the Client
// interface, so the server is tested with a fake and never reaches Google.
package gm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"runtime/debug"

	"github.com/rs/zerolog"
	"go.mau.fi/util/exhttp"

	"go.mau.fi/mautrix-gmessages/pkg/libgm"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"
)

const libgmModule = "go.mau.fi/mautrix-gmessages"

type Client interface {
	SetEventHandler(libgm.EventHandler)
	SetPairCallback(func(*gmproto.PairedData))
	SessionBlob() ([]byte, error)
	FetchConfig(ctx context.Context) error
	StartLogin(ctx context.Context) (string, error)
	RefreshPhoneRelay() (string, error)
	Connect(ctx context.Context) error
	Disconnect()
	Unpair(ctx context.Context) error
	SetActiveSession(ctx context.Context) error
	ListConversations(ctx context.Context, req *gmproto.ListConversationsRequest) (*gmproto.ListConversationsResponse, error)
	FetchMessages(ctx context.Context, conversationID string, count int64, cursor *gmproto.Cursor) (*gmproto.ListMessagesResponse, error)
	SendMessage(ctx context.Context, req *gmproto.SendMessageRequest) (*gmproto.SendMessageResponse, error)
	SendReaction(ctx context.Context, req *gmproto.SendReactionRequest) (*gmproto.SendReactionResponse, error)
	MarkRead(ctx context.Context, conversationID, messageID string) error
	DownloadMedia(ctx context.Context, mediaID string, key []byte) (io.ReadCloser, error)
	GetParticipantThumbnail(ctx context.Context, participantIDs ...string) (*gmproto.GetThumbnailResponse, error)
}

// Factory builds a client from a session blob (libgm AuthData JSON), or from fresh pairing keys
// when session is nil.
type Factory func(session []byte) (Client, error)

type libgmClient struct {
	*libgm.Client
}

func (c *libgmClient) SetPairCallback(fn func(*gmproto.PairedData)) { c.PairCallback.Store(&fn) }

func (c *libgmClient) SessionBlob() ([]byte, error) { return json.Marshal(c.AuthData) }

// NewFactory caps libgm's logger at Info: its Debug/Trace output includes message content.
func NewFactory(log zerolog.Logger) Factory {
	libLog := log.With().Str("component", "libgm").Logger().Level(max(log.GetLevel(), zerolog.InfoLevel))
	return func(session []byte) (Client, error) {
		ad := libgm.NewAuthData()
		if session != nil {
			ad = &libgm.AuthData{}
			if err := json.Unmarshal(session, ad); err != nil {
				return nil, fmt.Errorf("decode session: %w", err)
			}
		}
		return &libgmClient{Client: libgm.NewClient(ad, nil, libLog, exhttp.SensibleClientSettings)}, nil
	}
}

// LibgmVersion is the pinned mautrix-gmessages module version (a pseudo-version naming the commit).
func LibgmVersion() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, d := range bi.Deps {
		if d.Path == libgmModule {
			if d.Replace != nil {
				return d.Replace.Version
			}
			return d.Version
		}
	}
	return "unknown"
}
