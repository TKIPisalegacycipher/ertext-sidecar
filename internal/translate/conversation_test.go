package translate_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"

	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/pb"
	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/translate"
)

func TestConversation(t *testing.T) {
	in := &gmproto.Conversation{
		ConversationID:       "c1",
		Name:                 "Family",
		IsGroupChat:          true,
		Type:                 gmproto.ConversationType_RCS,
		LastMessageTimestamp: 1761300000000000,
		Unread:               true,
		Status:               gmproto.ConversationStatus_KEEP_ARCHIVED,
		ReadOnly:             false,
		Pinned:               true,
		DefaultOutgoingID:    "p-me",
		LatestMessageID:      "m9",
		AvatarHexColor:       "#ff0000",
		GroupAvatarURL:       "https://example.invalid/a.png",
		LatestMessage: &gmproto.LatestMessage{
			DisplayContent: "see you", FromMe: 1, DisplayName: "Me",
		},
		Participants: []*gmproto.Participant{
			{ID: &gmproto.SmallInfo{ParticipantID: "p-me", Number: "+15550001"}, IsMe: true, FullName: "Me"},
			{ID: &gmproto.SmallInfo{ParticipantID: "p2", Number: "+15550002"}, FullName: "Ann Lee", FirstName: "Ann",
				FormattedNumber: "(555) 000-2", AvatarHexColor: "#00ff00", ContactID: "k2", IsVisible: true},
		},
	}
	got := translate.Conversation(in)
	require.Equal(t, &pb.Conversation{
		Id: "c1", Name: "Family", IsGroup: true, Type: pb.ConversationType_CONVERSATION_TYPE_RCS,
		LastMessageTsUs: 1761300000000000, Unread: true,
		Status: pb.ConversationStatus_CONVERSATION_STATUS_ARCHIVED,
		Pinned: true, DefaultOutgoingId: "p-me", LatestMessageId: "m9",
		Snippet: "see you", SnippetFromMe: true, SnippetSender: "Me",
		AvatarHexColor: "#ff0000", GroupAvatarUrl: "https://example.invalid/a.png",
		Participants: []*pb.Participant{
			{Id: "p-me", FullName: "Me", FormattedNumber: "+15550001", IsMe: true},
			{Id: "p2", FullName: "Ann Lee", FirstName: "Ann", FormattedNumber: "(555) 000-2",
				AvatarHexColor: "#00ff00", ContactId: "k2", IsVisible: true},
		},
	}, got)
}

func TestConversationStatuses(t *testing.T) {
	cases := map[gmproto.ConversationStatus]pb.ConversationStatus{
		gmproto.ConversationStatus_ACTIVE:                      pb.ConversationStatus_CONVERSATION_STATUS_ACTIVE,
		gmproto.ConversationStatus_ARCHIVED:                    pb.ConversationStatus_CONVERSATION_STATUS_ARCHIVED,
		gmproto.ConversationStatus_DELETED:                     pb.ConversationStatus_CONVERSATION_STATUS_DELETED,
		gmproto.ConversationStatus_TRASH_FOLDER:                pb.ConversationStatus_CONVERSATION_STATUS_DELETED,
		gmproto.ConversationStatus_SPAM_FOLDER:                 pb.ConversationStatus_CONVERSATION_STATUS_SPAM,
		gmproto.ConversationStatus_BLOCKED_FOLDER:              pb.ConversationStatus_CONVERSATION_STATUS_BLOCKED,
		gmproto.ConversationStatus_CROSS_COUNTRY_FOLDER:        pb.ConversationStatus_CONVERSATION_STATUS_OTHER,
		gmproto.ConversationStatus_UNKNOWN_CONVERSATION_STATUS: pb.ConversationStatus_CONVERSATION_STATUS_UNSPECIFIED,
	}
	for in, want := range cases {
		got := translate.Conversation(&gmproto.Conversation{Status: in}).GetStatus()
		require.Equal(t, want, got, in.String())
	}
}
