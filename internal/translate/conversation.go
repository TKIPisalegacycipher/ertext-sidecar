package translate

import (
	"go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"

	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/pb"
)

func Participant(p *gmproto.Participant) *pb.Participant {
	number := p.GetFormattedNumber()
	if number == "" {
		number = p.GetID().GetNumber()
	}
	return &pb.Participant{
		Id:              p.GetID().GetParticipantID(),
		FullName:        p.GetFullName(),
		FirstName:       p.GetFirstName(),
		FormattedNumber: number,
		IsMe:            p.GetIsMe(),
		AvatarHexColor:  p.GetAvatarHexColor(),
		ContactId:       p.GetContactID(),
		IsVisible:       p.GetIsVisible(),
	}
}

func Conversation(c *gmproto.Conversation) *pb.Conversation {
	out := &pb.Conversation{
		Id:                c.GetConversationID(),
		Name:              c.GetName(),
		IsGroup:           c.GetIsGroupChat(),
		Type:              convType(c.GetType()),
		LastMessageTsUs:   c.GetLastMessageTimestamp(),
		Unread:            c.GetUnread(),
		Status:            convStatus(c.GetStatus()),
		ReadOnly:          c.GetReadOnly(),
		Pinned:            c.GetPinned(),
		DefaultOutgoingId: c.GetDefaultOutgoingID(),
		LatestMessageId:   c.GetLatestMessageID(),
		Snippet:           c.GetLatestMessage().GetDisplayContent(),
		SnippetFromMe:     c.GetLatestMessage().GetFromMe() != 0,
		SnippetSender:     c.GetLatestMessage().GetDisplayName(),
		AvatarHexColor:    c.GetAvatarHexColor(),
		GroupAvatarUrl:    c.GetGroupAvatarURL(),
	}
	for _, p := range c.GetParticipants() {
		out.Participants = append(out.Participants, Participant(p))
	}
	return out
}

func ConversationPage(resp *gmproto.ListConversationsResponse) *pb.ConversationPage {
	page := &pb.ConversationPage{}
	for _, c := range resp.GetConversations() {
		page.Conversations = append(page.Conversations, Conversation(c))
	}
	if c := resp.GetCursor(); c != nil && c.GetLastItemID() != "" && len(page.Conversations) > 0 {
		page.Next = cursor(c)
	}
	return page
}

// Contacts keeps the phone's contacts that have a number, in the phone's order.
func Contacts(cs []*gmproto.Contact) *pb.Contacts {
	out := &pb.Contacts{}
	for _, c := range cs {
		n := c.GetNumber()
		if n.GetNumber() == "" {
			continue
		}
		out.Contacts = append(out.Contacts, &pb.Contact{
			ContactId:       c.GetContactID(),
			Name:            c.GetName(),
			Number:          n.GetNumber(),
			FormattedNumber: n.GetFormattedNumber(),
			ParticipantId:   c.GetParticipantID(),
		})
	}
	return out
}
