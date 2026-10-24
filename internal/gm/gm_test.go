package gm

import (
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

var _ Client = (*libgmClient)(nil)

func TestFactoryFreshAndRoundTrip(t *testing.T) {
	f := NewFactory(zerolog.Nop())
	fresh, err := f(nil)
	require.NoError(t, err)
	blob, err := fresh.SessionBlob()
	require.NoError(t, err)
	require.Contains(t, string(blob), "request_crypto") // NewAuthData filled the pairing keys

	again, err := f(blob)
	require.NoError(t, err)
	blob2, err := again.SessionBlob()
	require.NoError(t, err)
	require.JSONEq(t, string(blob), string(blob2))
}

func TestFactoryRejectsGarbage(t *testing.T) {
	_, err := NewFactory(zerolog.Nop())([]byte("not json"))
	require.Error(t, err)
}

func TestLibgmVersionNonEmpty(t *testing.T) {
	require.NotEmpty(t, LibgmVersion())
}
