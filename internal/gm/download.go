package gm

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	"go.mau.fi/mautrix-gmessages/pkg/libgm"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/crypto"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/util"
)

// HTTPStatusError is a media download the server answered with a non-2xx status.
type HTTPStatusError struct{ Status int }

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("media download failed: HTTP %d", e.Status)
}

// fetchMedia is libgm's Client.DownloadMedia, plus a status check: libgm hands any response body
// to the decryptor, so an error page (403, 404, ...) surfaces as a decrypt error. The URL is a
// parameter so tests can point it at a local server.
func fetchMedia(ctx context.Context, hc *http.Client, url string, ad *libgm.AuthData, mediaID string, key []byte) (io.ReadCloser, error) {
	cryptor, err := crypto.NewAESGCMHelper(key)
	if err != nil {
		return nil, err
	}
	meta, err := proto.Marshal(&gmproto.DownloadAttachmentRequest{
		Info: &gmproto.AttachmentInfo{AttachmentID: mediaID, Encrypted: true},
		AuthData: &gmproto.AuthMessage{
			RequestID:        uuid.NewString(),
			TachyonAuthToken: ad.TachyonAuthToken,
			Network:          ad.AuthNetwork(),
			ConfigVersion:    util.ConfigMessage,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal download request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare request: %w", err)
	}
	util.BuildUploadHeaders(req, base64.StdEncoding.EncodeToString(meta))
	res, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		_ = res.Body.Close()
		return nil, &HTTPStatusError{Status: res.StatusCode}
	}
	return cryptor.DecryptStream(res.Body), nil
}
