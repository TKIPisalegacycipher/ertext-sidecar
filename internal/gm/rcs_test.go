package gm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mau.fi/mautrix-gmessages/pkg/libgm"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"
	"google.golang.org/protobuf/proto"

	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/gmext"
)

// The linknames resolve to libgm's internals and behave as expected, with a nil client only: no
// connection is ever made. A libgm bump that renames them fails here (or fails to build).
func TestLinknamesReachLibgm(t *testing.T) {
	require.Nil(t, sessionHandlerOf(nil))
	_, err := sendWithParams(nil, context.Background(), libgm.SendMessageParams{
		Action: gmproto.ActionType_GET_CONTACT_RCS_GROUP_STATUS,
	})
	require.ErrorIs(t, err, libgm.ErrClientIsNil)
}

func TestRCSRequestWireFormat(t *testing.T) {
	b, err := proto.Marshal(&gmext.GetContactRcsGroupStatusRequest{
		Number: &gmext.ContactNumber{Type: 7, Number: "1", AltNumber: "1"},
	})
	require.NoError(t, err)
	// {1: {1: 7, 2: "1", 3: "1"}}: the web's request for a typed number, no SIM, no conversation.
	require.Equal(t, []byte{0x0a, 0x08, 0x08, 0x07, 0x12, 0x01, '1', 0x1a, 0x01, '1'}, b)
}

func TestRCSStatusDecodes(t *testing.T) {
	got, err := decodeRCSStatus([]byte{0x08, 0x01})
	require.NoError(t, err)
	require.True(t, got.GetRcsGroupCapable())
	require.False(t, got.GetGroupEncryptionCapable())
	got, err = decodeRCSStatus([]byte{0x08, 0x01, 0x10, 0x01})
	require.NoError(t, err)
	require.True(t, got.GetGroupEncryptionCapable())
	got, err = decodeRCSStatus(nil)
	require.NoError(t, err, "an empty answer is both false")
	require.False(t, got.GetRcsGroupCapable())
	_, err = decodeRCSStatus([]byte{0xff})
	require.Error(t, err)
}
