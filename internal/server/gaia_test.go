package server

import (
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mau.fi/mautrix-gmessages/pkg/libgm"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/events"

	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/pb"
)

var testCookies = map[string]string{"SID": "s", "OSID": "o"}

func TestGoogleLoginSuccess(t *testing.T) {
	f := newFake()
	f.gaiaEmoji = []string{"🦊"}
	h := start(t, f.factory())
	h.send(req(1, &pb.StartGoogleLogin{Cookies: testCookies}))
	require.NotNil(t, h.response(1).GetAck())

	var got []*pb.Event // events that arrived before the Ack, in order
	for _, e := range h.events {
		if !isHello(e) {
			got = append(got, e)
		}
	}
	require.Len(t, got, 2)
	require.Equal(t, "🦊", got[0].GetPairEmoji().GetEmoji())
	require.Equal(t, []byte(`{"fake":true}`), got[1].GetPairSuccess().GetSessionBlob())

	require.Nil(t, f.sessionArg(), "pairing starts from fresh AuthData")
	require.Equal(t, testCookies, f.gotCookies())
	require.Equal(t, []string{"new", "cookies", "config", "gaia", "blob", "disconnect"}, f.Calls())

	h.send(req(2, &pb.SetActive{}))
	require.Equal(t, pb.ErrorCode_ERROR_CODE_NOT_CONNECTED, h.response(2).GetError().GetCode(),
		"the pairing client does not stay current")
	require.NoError(t, h.close())
}

func TestGoogleLoginRequiresCookies(t *testing.T) {
	f := newFake()
	h := start(t, f.factory())
	h.send(req(1, &pb.StartGoogleLogin{}))
	require.Equal(t, pb.ErrorCode_ERROR_CODE_BAD_REQUEST, h.response(1).GetError().GetCode())
	require.Empty(t, f.Calls())
	require.NoError(t, h.close())
}

func TestGoogleLoginErrors(t *testing.T) {
	http401 := events.HTTPError{Action: "signing in", Resp: &http.Response{StatusCode: http.StatusUnauthorized}}
	multi := fmt.Errorf("%w (%w)", libgm.ErrPairingInitTimeout, libgm.ErrHadMultipleDevices)
	for _, tc := range []struct {
		name string
		call string
		err  error
		want pb.ErrorCode
	}{
		{"incorrect emoji", "gaia", libgm.ErrIncorrectEmoji, pb.ErrorCode_ERROR_CODE_PAIRING_REJECTED},
		{"cancelled", "gaia", fmt.Errorf("%w (not me)", libgm.ErrPairingCancelled), pb.ErrorCode_ERROR_CODE_PAIRING_REJECTED},
		{"no devices", "gaia", libgm.ErrNoDevicesFound, pb.ErrorCode_ERROR_CODE_NO_PHONE},
		{"phone not found", "gaia", libgm.ErrPhoneNotFound, pb.ErrorCode_ERROR_CODE_NO_PHONE},
		{"no cookies", "gaia", libgm.ErrNoCookies, pb.ErrorCode_ERROR_CODE_NOT_SIGNED_IN},
		{"invalid credentials", "gaia", fmt.Errorf("prepare: %w", events.ErrInvalidCredentials), pb.ErrorCode_ERROR_CODE_NOT_SIGNED_IN},
		{"http 401", "gaia", fmt.Errorf("prepare: %w", http401), pb.ErrorCode_ERROR_CODE_NOT_SIGNED_IN},
		{"config 401", "config", fmt.Errorf("failed to fetch config: %w", http401), pb.ErrorCode_ERROR_CODE_NOT_SIGNED_IN},
		{"pairing timeout", "gaia", fmt.Errorf("%w (code: 1/2)", libgm.ErrPairingTimeout), pb.ErrorCode_ERROR_CODE_TIMEOUT},
		{"init timeout", "gaia", libgm.ErrPairingInitTimeout, pb.ErrorCode_ERROR_CODE_TIMEOUT},
		{"multiple devices", "gaia", multi, pb.ErrorCode_ERROR_CODE_INTERNAL},
		{"other", "gaia", errors.New("boom"), pb.ErrorCode_ERROR_CODE_INTERNAL},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFake()
			f.err[tc.call] = tc.err
			h := start(t, f.factory())
			h.send(req(1, &pb.StartGoogleLogin{Cookies: testCookies}))
			e := h.response(1).GetError()
			require.Equal(t, tc.want, e.GetCode())
			require.Contains(t, e.GetMessage(), tc.err.Error())
			require.Contains(t, f.Calls(), "disconnect")
			for _, ev := range h.events {
				require.Nil(t, ev.GetPairSuccess())
			}
			h.send(req(2, &pb.SetActive{}))
			require.Equal(t, pb.ErrorCode_ERROR_CODE_NOT_CONNECTED, h.response(2).GetError().GetCode())
			require.NoError(t, h.close())
		})
	}
}

func TestGoogleLoginSupersededByConnect(t *testing.T) {
	pairing := newFake()
	pairing.gaiaEmoji = []string{"🦊"}
	pairing.gaiaGate = make(chan struct{})
	next := newFake()
	var n atomic.Int32
	h := start(t, func(s []byte) (gmClient, error) {
		if n.Add(1) == 1 {
			return pairing.factory()(s)
		}
		return next.factory()(s)
	})
	h.send(req(1, &pb.StartGoogleLogin{Cookies: testCookies}))
	h.event(func(e *pb.Event) bool { return e.GetPairEmoji() != nil })

	h.send(req(2, &pb.Connect{SessionBlob: []byte("s")}))
	require.NotNil(t, h.response(2).GetAck())
	require.Contains(t, pairing.Calls(), "disconnect")

	close(pairing.gaiaGate) // pairing "succeeds" after it was superseded
	e := h.response(1).GetError()
	require.NotNil(t, e)
	require.Contains(t, e.GetMessage(), "superseded")
	for _, ev := range h.events {
		require.Nil(t, ev.GetPairSuccess())
	}
	require.NotContains(t, next.Calls(), "disconnect", "the newer client stays connected")
	h.send(req(3, &pb.SetActive{}))
	require.NotNil(t, h.response(3).GetAck())
	require.NoError(t, h.close())
}
