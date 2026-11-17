package translate

import "go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"

// Since about 2026-11-12 Google sends some conversations and messages with the top-level ids
// empty and the ids only in nested fields. libgm fills them in for the responses it decrypts
// (mautrix-gmessages b580a1a, #166); these helpers do the same for anything that reaches
// translate without them, so no conversation or message leaves the sidecar without an id.

func conversationID(c *gmproto.Conversation) string {
	if id := c.GetConversationID(); id != "" {
		return id
	}
	return c.GetSomeKindOfGroupID().GetConversationID()
}

func latestMessageID(c *gmproto.Conversation) string {
	if id := c.GetLatestMessageID(); id != "" {
		return id
	}
	return c.GetLatestMessageAnotherID().GetMessageID()
}

func messageID(m *gmproto.Message) string {
	if id := m.GetMessageID(); id != "" {
		return id
	}
	return m.GetAnotherMessageID().GetMessageID()
}

func messageConversationID(m *gmproto.Message) string {
	if id := m.GetConversationID(); id != "" {
		return id
	}
	return m.GetSomeKindOfGroupID().GetConversationID()
}
