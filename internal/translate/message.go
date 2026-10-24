package translate

import (
	"slices"
	"strings"

	"go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"

	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/pb"
)

func Message(m *gmproto.Message) *pb.Message {
	st := m.GetMessageStatus().GetStatus()
	out := &pb.Message{
		Id:                  m.GetMessageID(),
		TmpId:               m.GetTmpID(),
		ConversationId:      m.GetConversationID(),
		SenderParticipantId: m.GetParticipantID(),
		TimestampUs:         m.GetTimestamp(),
		Kind:                kind(m.GetType(), st),
		Status:              Status(st),
		RawStatus:           int32(st),
		Subject:             m.GetSubject(),
		ReplyToId:           m.GetReplyMessage().GetMessageID(),
		FromMe:              isOutgoing(st),
	}
	var texts []string
	for _, info := range m.GetMessageInfo() {
		switch d := info.GetData().(type) {
		case *gmproto.MessageInfo_MessageContent:
			if t := d.MessageContent.GetContent(); t != "" {
				texts = append(texts, t)
			}
		case *gmproto.MessageInfo_MediaContent:
			out.Media = append(out.Media, media(d.MediaContent, info.GetActionMessageID()))
		}
	}
	out.Text = strings.Join(texts, "\n")
	for _, r := range m.GetReactions() {
		out.Reactions = append(out.Reactions, &pb.Reaction{
			Emoji:          r.GetData().GetUnicode(),
			ParticipantIds: r.GetParticipantIDs(),
		})
	}
	return out
}

func media(c *gmproto.MediaContent, actionID string) *pb.Media {
	return &pb.Media{
		MediaId:         c.GetMediaID(),
		Key:             c.GetDecryptionKey(),
		ThumbMediaId:    c.GetThumbnailMediaID(),
		ThumbKey:        c.GetThumbnailDecryptionKey(),
		Mime:            c.GetMimeType(),
		Name:            c.GetMediaName(),
		Size:            c.GetSize(),
		Width:           c.GetDimensions().GetWidth(),
		Height:          c.GetDimensions().GetHeight(),
		ActionMessageId: actionID,
	}
}

// MessagePage normalises libgm's paging: libgm returns newest first, its cursor timestamp is in
// milliseconds while message timestamps are microseconds, and a backward page can include items at
// or after the cursor. The result is oldest first, strictly older than reqCursor, with Next set
// whenever the page is non-empty (an empty page = fully backfilled).
func MessagePage(resp *gmproto.ListMessagesResponse, reqCursor *gmproto.Cursor) *pb.MessagePage {
	msgs := slices.Clone(resp.GetMessages())
	slices.Reverse(msgs)
	page := &pb.MessagePage{TotalMessages: resp.GetTotalMessages()}
	var oldest *gmproto.Message
	for _, m := range msgs {
		if reqCursor != nil && m.GetTimestamp()/1000 >= reqCursor.GetLastItemTimestamp() {
			continue
		}
		if oldest == nil {
			oldest = m
		}
		page.Messages = append(page.Messages, Message(m))
	}
	if oldest == nil {
		return page
	}
	if c := resp.GetCursor(); c != nil && c.GetLastItemID() != "" {
		page.Next = cursor(c)
	} else {
		page.Next = &pb.Cursor{LastItemId: oldest.GetMessageID(), LastItemTsMs: oldest.GetTimestamp() / 1000}
	}
	return page
}
