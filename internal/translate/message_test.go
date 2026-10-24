package translate_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"
	"google.golang.org/protobuf/proto"

	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/pb"
	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/translate"
)

func gmMsg(id string, tsUs int64, st gmproto.MessageStatusType) *gmproto.Message {
	return &gmproto.Message{
		MessageID: id, ConversationID: "c1", ParticipantID: "p2", Timestamp: tsUs, Type: 4,
		MessageStatus: &gmproto.MessageStatus{Status: st},
	}
}

func TestMessageFull(t *testing.T) {
	in := &gmproto.Message{
		MessageID: "m1", TmpID: "tmp_1", ConversationID: "c1", ParticipantID: "p-me",
		Timestamp: 1761300000000000, Type: 4,
		MessageStatus: &gmproto.MessageStatus{Status: gmproto.MessageStatusType_OUTGOING_DELIVERED},
		Subject:       proto.String("Re: plans"),
		ReplyMessage:  &gmproto.ReplyMessage{MessageID: "m0"},
		MessageInfo: []*gmproto.MessageInfo{
			{Data: &gmproto.MessageInfo_MessageContent{MessageContent: &gmproto.MessageContent{Content: "line one"}}},
			{ActionMessageID: proto.String("a1"), Data: &gmproto.MessageInfo_MediaContent{MediaContent: &gmproto.MediaContent{
				MediaID: "md1", DecryptionKey: []byte{1, 2}, ThumbnailMediaID: "md1t", ThumbnailDecryptionKey: []byte{3},
				MimeType: "image/jpeg", MediaName: "pic.jpg", Size: 1234,
				Dimensions: &gmproto.Dimensions{Width: 640, Height: 480},
			}}},
			{Data: &gmproto.MessageInfo_MessageContent{MessageContent: &gmproto.MessageContent{Content: "line two"}}},
		},
		Reactions: []*gmproto.ReactionEntry{
			{Data: &gmproto.ReactionData{Unicode: "👍"}, ParticipantIDs: []string{"p2"}},
		},
	}
	got := translate.Message(in)
	require.Equal(t, &pb.Message{
		Id: "m1", TmpId: "tmp_1", ConversationId: "c1", SenderParticipantId: "p-me",
		TimestampUs: 1761300000000000, Kind: pb.MessageKind_MESSAGE_KIND_RCS,
		Status: pb.MessageStatus_MESSAGE_STATUS_DELIVERED, RawStatus: 2,
		Text: "line one\nline two", Subject: "Re: plans", ReplyToId: "m0", FromMe: true,
		Media: []*pb.Media{{
			MediaId: "md1", Key: []byte{1, 2}, ThumbMediaId: "md1t", ThumbKey: []byte{3},
			Mime: "image/jpeg", Name: "pic.jpg", Size: 1234, Width: 640, Height: 480, ActionMessageId: "a1",
		}},
		Reactions: []*pb.Reaction{{Emoji: "👍", ParticipantIds: []string{"p2"}}},
	}, got)
}

func TestMessageKind(t *testing.T) {
	sms := gmMsg("a", 1, gmproto.MessageStatusType_INCOMING_COMPLETE)
	sms.Type = 1
	require.Equal(t, pb.MessageKind_MESSAGE_KIND_SMS, translate.Message(sms).Kind)
	mms := gmMsg("b", 1, gmproto.MessageStatusType_INCOMING_COMPLETE)
	mms.Type = 3
	require.Equal(t, pb.MessageKind_MESSAGE_KIND_MMS, translate.Message(mms).Kind)
	sys := gmMsg("c", 1, gmproto.MessageStatusType_TOMBSTONE_PARTICIPANT_LEFT)
	sys.Type = 1
	require.Equal(t, pb.MessageKind_MESSAGE_KIND_SYSTEM, translate.Message(sys).Kind)
	require.False(t, translate.Message(sys).FromMe)
}

func TestMessagePageNewestPageReversesToOldestFirst(t *testing.T) {
	resp := &gmproto.ListMessagesResponse{
		Messages: []*gmproto.Message{
			gmMsg("m3", 3_000_000, gmproto.MessageStatusType_INCOMING_COMPLETE),
			gmMsg("m2", 2_000_000, gmproto.MessageStatusType_INCOMING_COMPLETE),
			gmMsg("m1", 1_000_000, gmproto.MessageStatusType_INCOMING_COMPLETE),
		},
		TotalMessages: 10,
		Cursor:        &gmproto.Cursor{LastItemID: "m1", LastItemTimestamp: 1_000},
	}
	page := translate.MessagePage(resp, nil)
	require.Equal(t, []string{"m1", "m2", "m3"}, ids(page.Messages))
	require.Equal(t, &pb.Cursor{LastItemId: "m1", LastItemTsMs: 1_000}, page.Next)
	require.EqualValues(t, 10, page.TotalMessages)
}

func TestMessagePageFiltersAtOrAfterCursorAndDerivesNext(t *testing.T) {
	resp := &gmproto.ListMessagesResponse{Messages: []*gmproto.Message{
		gmMsg("m3", 3_000_000, gmproto.MessageStatusType_INCOMING_COMPLETE), // == cursor: drop
		gmMsg("m2", 2_000_000, gmproto.MessageStatusType_INCOMING_COMPLETE),
		gmMsg("m1", 1_500_000, gmproto.MessageStatusType_INCOMING_COMPLETE),
	}} // no cursor in response
	page := translate.MessagePage(resp, &gmproto.Cursor{LastItemID: "m3", LastItemTimestamp: 3_000})
	require.Equal(t, []string{"m1", "m2"}, ids(page.Messages))
	require.Equal(t, &pb.Cursor{LastItemId: "m1", LastItemTsMs: 1_500}, page.Next)
}

func TestMessagePageEmptyMeansFullyBackfilled(t *testing.T) {
	page := translate.MessagePage(&gmproto.ListMessagesResponse{
		Cursor: &gmproto.Cursor{LastItemID: "x", LastItemTimestamp: 5},
	}, &gmproto.Cursor{LastItemID: "m1", LastItemTimestamp: 1_000})
	require.Empty(t, page.Messages)
	require.Nil(t, page.Next)
}

func TestConversationPage(t *testing.T) {
	page := translate.ConversationPage(&gmproto.ListConversationsResponse{
		Conversations: []*gmproto.Conversation{{ConversationID: "c1"}, {ConversationID: "c2"}},
		Cursor:        &gmproto.Cursor{LastItemID: "c2", LastItemTimestamp: 77},
	})
	require.Len(t, page.Conversations, 2)
	require.Equal(t, &pb.Cursor{LastItemId: "c2", LastItemTsMs: 77}, page.Next)
	require.Nil(t, translate.ConversationPage(&gmproto.ListConversationsResponse{}).Next)
}

func ids(ms []*pb.Message) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.Id)
	}
	return out
}
