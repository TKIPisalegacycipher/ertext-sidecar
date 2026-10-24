package gm

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mau.fi/mautrix-gmessages/pkg/libgm"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/crypto"
)

func serve(t *testing.T, status int, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NotEmpty(t, r.Header.Get("x-goog-download-metadata"))
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFetchMediaReportsNon2xxAsHTTPErrorNotDecryptError(t *testing.T) {
	srv := serve(t, http.StatusForbidden, []byte("<html>denied</html>"))
	key := make([]byte, 32)
	rc, err := fetchMedia(context.Background(), srv.Client(), srv.URL, libgm.NewAuthData(), "md1", key)
	require.Nil(t, rc)
	var he *HTTPStatusError
	require.ErrorAs(t, err, &he)
	require.Equal(t, http.StatusForbidden, he.Status)
	require.Contains(t, err.Error(), "403")
	require.NotContains(t, err.Error(), "denied") // no body content in errors
}

func TestFetchMediaDecryptsA2xxBody(t *testing.T) {
	key := make([]byte, 32)
	h, err := crypto.NewAESGCMHelper(key)
	require.NoError(t, err)
	enc, err := h.EncryptData([]byte("pixels"))
	require.NoError(t, err)
	srv := serve(t, http.StatusOK, enc)
	rc, err := fetchMedia(context.Background(), srv.Client(), srv.URL, libgm.NewAuthData(), "md1", key)
	require.NoError(t, err)
	defer rc.Close()
	got, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.Equal(t, "pixels", string(got))
}
