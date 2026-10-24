package translate_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"

	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/pb"
	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/translate"
)

func TestStatus(t *testing.T) {
	cases := map[gmproto.MessageStatusType]pb.MessageStatus{
		gmproto.MessageStatusType_OUTGOING_YET_TO_SEND:            pb.MessageStatus_MESSAGE_STATUS_PENDING,
		gmproto.MessageStatusType_OUTGOING_SENDING:                pb.MessageStatus_MESSAGE_STATUS_PENDING,
		gmproto.MessageStatusType_OUTGOING_COMPLETE:               pb.MessageStatus_MESSAGE_STATUS_SENT,
		gmproto.MessageStatusType_OUTGOING_DELIVERED:              pb.MessageStatus_MESSAGE_STATUS_DELIVERED,
		gmproto.MessageStatusType_OUTGOING_DISPLAYED:              pb.MessageStatus_MESSAGE_STATUS_READ,
		gmproto.MessageStatusType_OUTGOING_FAILED_GENERIC:         pb.MessageStatus_MESSAGE_STATUS_FAILED,
		gmproto.MessageStatusType_OUTGOING_FAILED_TOO_LARGE:       pb.MessageStatus_MESSAGE_STATUS_FAILED,
		gmproto.MessageStatusType_OUTGOING_CANCELED:               pb.MessageStatus_MESSAGE_STATUS_FAILED,
		gmproto.MessageStatusType_OUTGOING_FAILED_TO_ENCRYPT:      pb.MessageStatus_MESSAGE_STATUS_FAILED,
		gmproto.MessageStatusType_OUTGOING_DELETED:                pb.MessageStatus_MESSAGE_STATUS_DELETED,
		gmproto.MessageStatusType_INCOMING_COMPLETE:               pb.MessageStatus_MESSAGE_STATUS_INCOMING,
		gmproto.MessageStatusType_INCOMING_DISPLAYED:              pb.MessageStatus_MESSAGE_STATUS_INCOMING,
		gmproto.MessageStatusType_INCOMING_AUTO_DOWNLOADING:       pb.MessageStatus_MESSAGE_STATUS_INCOMING_DOWNLOADING,
		gmproto.MessageStatusType_INCOMING_YET_TO_MANUAL_DOWNLOAD: pb.MessageStatus_MESSAGE_STATUS_INCOMING_DOWNLOADING,
		gmproto.MessageStatusType_INCOMING_DELETED:                pb.MessageStatus_MESSAGE_STATUS_DELETED,
		gmproto.MessageStatusType_MESSAGE_DELETED:                 pb.MessageStatus_MESSAGE_STATUS_DELETED,
		gmproto.MessageStatusType_TOMBSTONE_PARTICIPANT_JOINED:    pb.MessageStatus_MESSAGE_STATUS_INCOMING,
		gmproto.MessageStatusType_STATUS_UNKNOWN:                  pb.MessageStatus_MESSAGE_STATUS_UNSPECIFIED,
	}
	for in, want := range cases {
		require.Equal(t, want, translate.Status(in), in.String())
	}
	require.True(t, translate.IsDeleted(gmproto.MessageStatusType_INCOMING_DELETED))
	require.True(t, translate.IsDeleted(gmproto.MessageStatusType_MESSAGE_DELETED))
	require.False(t, translate.IsDeleted(gmproto.MessageStatusType_INCOMING_COMPLETE))
}
