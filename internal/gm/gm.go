// Package gm is the only place that touches *libgm.Client. Everything else depends on the Client
// interface, so the server is tested with a fake and never reaches Google.
package gm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime/debug"

	"github.com/rs/zerolog"
	"go.mau.fi/util/exhttp"

	"go.mau.fi/mautrix-gmessages/pkg/libgm"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/util"

	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/gmext"
)

const libgmModule = "go.mau.fi/mautrix-gmessages"

type Client interface {
	SetEventHandler(libgm.EventHandler)
	SetPairCallback(func(*gmproto.PairedData))
	SessionBlob() ([]byte, error)
	FetchConfig(ctx context.Context) error
	// SetCookies sets the Google account cookies used by Google-account pairing.
	SetCookies(map[string]string)
	// DoGaiaPairing runs Google-account pairing, reporting the emoji to tap on the phone, and
	// returns once the phone has confirmed or rejected it.
	DoGaiaPairing(ctx context.Context, emojiCallback func(string)) error
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
	UpdateConversation(ctx context.Context, req *gmproto.UpdateConversationRequest) (*gmproto.UpdateConversationResponse, error)
	DeleteConversation(ctx context.Context, conversationID, phone string) error
	GetOrCreateConversation(ctx context.Context, req *gmproto.GetOrCreateConversationRequest) (*gmproto.GetOrCreateConversationResponse, error)
	ListContacts(ctx context.Context) (*gmproto.ListContactsResponse, error)
	ListTopContacts(ctx context.Context) (*gmproto.ListTopContactsResponse, error)
	// GetContactRCSGroupStatus is ActionType 26, sent through libgm's internals (rcs.go).
	GetContactRCSGroupStatus(ctx context.Context, req *gmext.GetContactRcsGroupStatusRequest) (*gmext.GetContactRcsGroupStatusResponse, error)
	DownloadMedia(ctx context.Context, mediaID string, key []byte) (io.ReadCloser, error)
	GetFullSizeImage(ctx context.Context, messageID, actionMessageID string) (*gmproto.GetFullSizeImageResponse, error)
	GetParticipantThumbnail(ctx context.Context, participantIDs ...string) (*gmproto.GetThumbnailResponse, error)
}

// Factory builds a client from a session blob (libgm AuthData JSON), or from fresh pairing keys
// when session is nil.
type Factory func(session []byte) (Client, error)

type libgmClient struct {
	*libgm.Client
	dl *http.Client // media downloads
}

// DownloadMedia shadows libgm's so a non-2xx response is reported as one (see fetchMedia). libgm
// builds its own HTTP client from the same settings, with no cookies or other state.
func (c *libgmClient) DownloadMedia(ctx context.Context, mediaID string, key []byte) (io.ReadCloser, error) {
	return fetchMedia(ctx, c.dl, util.UploadMediaURL, c.AuthData, mediaID, key)
}

func (c *libgmClient) SetPairCallback(fn func(*gmproto.PairedData)) { c.PairCallback.Store(&fn) }

func (c *libgmClient) SetCookies(cookies map[string]string) { c.AuthData.SetCookies(cookies) }

// DoGaiaPairing is libgm's DoGaiaPairing minus its tail, which fires PairSuccessful and starts a
// background Reconnect on context.TODO(). The server disconnects the pairing client and Rust
// reconnects from the session blob (as mautrix-gmessages' finishLogin does), so that Reconnect
// would only race the Disconnect and could leave a long poll running. The long poll that pairing
// needs lives on ctx, like the bridge's bgCtx.
func (c *libgmClient) DoGaiaPairing(ctx context.Context, emojiCallback func(string)) error {
	emoji, ps, err := c.StartGaiaPairing(ctx, ctx)
	if err != nil {
		return err
	}
	emojiCallback(emoji)
	_, err = c.FinishGaiaPairing(ctx, ps)
	return err
}

func (c *libgmClient) SessionBlob() ([]byte, error) { return json.Marshal(c.AuthData) }

// libgmLevel caps libgm's logger at Error: its Debug/Trace output includes message content, and
// it logs base64 decrypted payloads at Warn (event_handler.go, "Got unknown event type").
func libgmLevel(parent zerolog.Level) zerolog.Level { return max(parent, zerolog.ErrorLevel) }

// NewFactory builds libgm clients whose logger is capped by libgmLevel.
func NewFactory(log zerolog.Logger) Factory {
	libLog := log.With().Str("component", "libgm").Logger().Level(libgmLevel(log.GetLevel()))
	return func(session []byte) (Client, error) {
		ad := libgm.NewAuthData()
		if session != nil {
			ad = &libgm.AuthData{}
			if err := json.Unmarshal(session, ad); err != nil {
				return nil, fmt.Errorf("decode session: %w", err)
			}
		}
		return &libgmClient{
			Client: libgm.NewClient(ad, nil, libLog, exhttp.SensibleClientSettings),
			dl:     exhttp.SensibleClientSettings.Compile(),
		}, nil
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
