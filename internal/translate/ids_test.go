package translate_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mau.fi/mautrix-gmessages/pkg/libgm"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"

	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/translate"
)

// Google has sent conversations and messages whose top-level ids are empty, with the ids only in
// nested fields (mautrix-gmessages #166). Each of these used to reach Rust with an empty id, where
// it was dropped, so the conversation list came back empty and no new message was stored.

func nestedOnlyConversation() *gmproto.Conversation {
	return &gmproto.Conversation{
		SomeKindOfGroupID:      &gmproto.SomeKindOfGroupID{ConversationID: "c-nested"},
		LatestMessageAnotherID: &gmproto.AnotherMessageID{MessageID: "m-latest"},
	}
}

func nestedOnlyMessage(id string, ts int64) *gmproto.Message {
	return &gmproto.Message{
		AnotherMessageID:  &gmproto.AnotherMessageID{MessageID: id},
		SomeKindOfGroupID: &gmproto.SomeKindOfGroupID{ConversationID: "c-nested"},
		Timestamp:         ts,
	}
}

func TestConversationFallsBackToNestedIDs(t *testing.T) {
	got := translate.Conversation(nestedOnlyConversation())
	require.Equal(t, "c-nested", got.GetId())
	require.Equal(t, "m-latest", got.GetLatestMessageId())
}

func TestConversationPrefersTopLevelIDs(t *testing.T) {
	c := nestedOnlyConversation()
	c.ConversationID, c.LatestMessageID = "c-top", "m-top"
	got := translate.Conversation(c)
	require.Equal(t, "c-top", got.GetId())
	require.Equal(t, "m-top", got.GetLatestMessageId())
}

func TestConversationPageKeepsNestedOnlyConversations(t *testing.T) {
	page := translate.ConversationPage(&gmproto.ListConversationsResponse{
		Conversations: []*gmproto.Conversation{nestedOnlyConversation()},
	})
	require.Len(t, page.GetConversations(), 1)
	require.Equal(t, "c-nested", page.GetConversations()[0].GetId())
}

func TestMessageFallsBackToNestedIDs(t *testing.T) {
	got := translate.Message(nestedOnlyMessage("m-nested", 1_000_000))
	require.Equal(t, "m-nested", got.GetId())
	require.Equal(t, "c-nested", got.GetConversationId())
}

func TestMessagePrefersTopLevelIDs(t *testing.T) {
	m := nestedOnlyMessage("m-nested", 1_000_000)
	m.MessageID, m.ConversationID = "m-top", "c-top"
	got := translate.Message(m)
	require.Equal(t, "m-top", got.GetId())
	require.Equal(t, "c-top", got.GetConversationId())
}

func TestMessagePageCursorUsesNestedID(t *testing.T) {
	// No cursor from libgm, so Next comes from the oldest message's id.
	page := translate.MessagePage(&gmproto.ListMessagesResponse{
		Messages: []*gmproto.Message{nestedOnlyMessage("m-new", 2_000_000), nestedOnlyMessage("m-old", 1_000_000)},
	}, nil)
	require.Len(t, page.GetMessages(), 2)
	require.Equal(t, "m-old", page.GetNext().GetLastItemId())
}

func TestLiveMessageEventsFallBackToNestedIDs(t *testing.T) {
	up := translate.Event(&libgm.WrappedMessage{Message: nestedOnlyMessage("m-live", 1_000_000)})
	require.Len(t, up, 1)
	got := up[0].GetMessageUpserted().GetMessage()
	require.Equal(t, "m-live", got.GetId())
	require.Equal(t, "c-nested", got.GetConversationId())

	gone := nestedOnlyMessage("m-gone", 1_000_000)
	gone.MessageStatus = &gmproto.MessageStatus{Status: gmproto.MessageStatusType_MESSAGE_DELETED}
	del := translate.Event(&libgm.WrappedMessage{Message: gone})
	require.Len(t, del, 1)
	require.Equal(t, "m-gone", del[0].GetMessageDeleted().GetMessageId())
	require.Equal(t, "c-nested", del[0].GetMessageDeleted().GetConversationId())
}

func TestLiveConversationEventsFallBackToNestedIDs(t *testing.T) {
	up := translate.Event(nestedOnlyConversation())
	require.Len(t, up, 1)
	require.Equal(t, "c-nested", up[0].GetConversationUpserted().GetConversation().GetId())
}
