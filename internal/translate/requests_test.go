package translate_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"

	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/pb"
	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/translate"
)

func TestSendMessageRequest(t *testing.T) {
	got, err := translate.SendMessageRequest(&pb.SendMessage{
		ConversationId: "c1", OutgoingParticipantId: "p-me", Sim: &pb.Sim{SimNumber: 2, PayloadKind: 1},
		Text: "hi", ReplyToId: "m0", TmpId: "tmp_1",
	})
	require.NoError(t, err)
	require.Equal(t, "c1", got.ConversationID)
	require.Equal(t, "tmp_1", got.TmpID)
	require.Equal(t, "tmp_1", got.MessagePayload.TmpID)
	require.Equal(t, "tmp_1", got.MessagePayload.TmpID2)
	require.Equal(t, "c1", got.MessagePayload.ConversationID)
	require.Equal(t, "p-me", got.MessagePayload.ParticipantID)
	require.Equal(t, "hi", got.MessagePayload.MessageInfo[0].GetMessageContent().GetContent())
	require.Equal(t, int32(2), got.SIMPayload.GetSIMNumber())
	require.Equal(t, int32(1), got.SIMPayload.GetTwo())
	require.Equal(t, "m0", got.Reply.GetMessageID())

	noReply, err := translate.SendMessageRequest(&pb.SendMessage{ConversationId: "c1", TmpId: "t", Text: "x"})
	require.NoError(t, err)
	require.Nil(t, noReply.Reply)
	require.Nil(t, noReply.SIMPayload)
}

func TestSendMessageRequestValidation(t *testing.T) {
	for _, bad := range []*pb.SendMessage{
		{TmpId: "t", Text: "x"},            // no conversation
		{ConversationId: "c1", Text: "x"},  // no tmp id
		{ConversationId: "c1", TmpId: "t"}, // no text
	} {
		_, err := translate.SendMessageRequest(bad)
		require.ErrorIs(t, err, translate.ErrBadRequest)
	}
}

func TestSendReactionRequest(t *testing.T) {
	got, err := translate.SendReactionRequest(&pb.SendReaction{
		MessageId: "m1", Emoji: "👍", Action: pb.ReactionAction_REACTION_ACTION_ADD, Sim: &pb.Sim{SimNumber: 1},
	})
	require.NoError(t, err)
	require.Equal(t, "m1", got.MessageID)
	require.Equal(t, "👍", got.ReactionData.GetUnicode())
	require.Equal(t, gmproto.SendReactionRequest_ADD, got.Action)
	require.Equal(t, int32(1), got.SIMPayload.GetSIMNumber())

	rm, err := translate.SendReactionRequest(&pb.SendReaction{MessageId: "m1", Emoji: "👍", Action: pb.ReactionAction_REACTION_ACTION_REMOVE})
	require.NoError(t, err)
	require.Equal(t, gmproto.SendReactionRequest_REMOVE, rm.Action)

	_, err = translate.SendReactionRequest(&pb.SendReaction{MessageId: "m1", Emoji: "👍"})
	require.ErrorIs(t, err, translate.ErrBadRequest)
	_, err = translate.SendReactionRequest(&pb.SendReaction{Emoji: "👍", Action: pb.ReactionAction_REACTION_ACTION_ADD})
	require.ErrorIs(t, err, translate.ErrBadRequest)
}

func TestCursorAndFolder(t *testing.T) {
	require.Nil(t, translate.GMCursor(nil))
	require.Equal(t, &gmproto.Cursor{LastItemID: "x", LastItemTimestamp: 9},
		translate.GMCursor(&pb.Cursor{LastItemId: "x", LastItemTsMs: 9}))
	require.Equal(t, gmproto.ListConversationsRequest_INBOX, translate.Folder(pb.Folder_FOLDER_UNSPECIFIED))
	require.Equal(t, gmproto.ListConversationsRequest_INBOX, translate.Folder(pb.Folder_FOLDER_INBOX))
	require.Equal(t, gmproto.ListConversationsRequest_ARCHIVE, translate.Folder(pb.Folder_FOLDER_ARCHIVE))
}

func TestSims(t *testing.T) {
	got := translate.Sims(&gmproto.Settings{SIMCards: []*gmproto.SIMCard{{
		SIMData: &gmproto.SIMData{
			SIMPayload:  &gmproto.SIMPayload{Two: 1, SIMNumber: 1},
			CarrierName: "Mint", ColorHex: "#123456", FormattedPhoneNumber: "(555) 000-1",
		},
		SIMParticipant: &gmproto.SIMParticipant{ID: "p-me"},
	}}})
	require.Equal(t, &pb.SimsUpdated{Sims: []*pb.SimCard{{
		Sim: &pb.Sim{SimNumber: 1, PayloadKind: 1}, ParticipantId: "p-me",
		CarrierName: "Mint", FormattedNumber: "(555) 000-1", ColorHex: "#123456",
	}}}, got)
}
