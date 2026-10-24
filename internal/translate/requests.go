package translate

import (
	"errors"
	"fmt"

	"go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"

	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/pb"
)

var ErrBadRequest = errors.New("bad request")

func badRequest(what string) error { return fmt.Errorf("%w: %s", ErrBadRequest, what) }

func cursor(c *gmproto.Cursor) *pb.Cursor {
	return &pb.Cursor{LastItemId: c.GetLastItemID(), LastItemTsMs: c.GetLastItemTimestamp()}
}

func GMCursor(c *pb.Cursor) *gmproto.Cursor {
	if c == nil {
		return nil
	}
	return &gmproto.Cursor{LastItemID: c.GetLastItemId(), LastItemTimestamp: c.GetLastItemTsMs()}
}

func Folder(f pb.Folder) gmproto.ListConversationsRequest_Folder {
	if f == pb.Folder_FOLDER_ARCHIVE {
		return gmproto.ListConversationsRequest_ARCHIVE
	}
	return gmproto.ListConversationsRequest_INBOX
}

func simPayload(s *pb.Sim) *gmproto.SIMPayload {
	if s == nil {
		return nil
	}
	return &gmproto.SIMPayload{Two: s.GetPayloadKind(), SIMNumber: s.GetSimNumber()}
}

// SendMessageRequest mirrors how mautrix-gmessages builds a text send (handlematrix.go).
func SendMessageRequest(m *pb.SendMessage) (*gmproto.SendMessageRequest, error) {
	switch {
	case m.GetConversationId() == "":
		return nil, badRequest("conversation_id required")
	case m.GetTmpId() == "":
		return nil, badRequest("tmp_id required")
	case m.GetText() == "":
		return nil, badRequest("text required")
	}
	req := &gmproto.SendMessageRequest{
		ConversationID: m.GetConversationId(),
		MessagePayload: &gmproto.MessagePayload{
			TmpID:          m.GetTmpId(),
			ConversationID: m.GetConversationId(),
			ParticipantID:  m.GetOutgoingParticipantId(),
			TmpID2:         m.GetTmpId(),
			MessageInfo: []*gmproto.MessageInfo{{
				Data: &gmproto.MessageInfo_MessageContent{MessageContent: &gmproto.MessageContent{Content: m.GetText()}},
			}},
		},
		SIMPayload: simPayload(m.GetSim()),
		TmpID:      m.GetTmpId(),
	}
	if m.GetReplyToId() != "" {
		req.Reply = &gmproto.ReplyPayload{MessageID: m.GetReplyToId()}
	}
	return req, nil
}

func SendReactionRequest(r *pb.SendReaction) (*gmproto.SendReactionRequest, error) {
	var action gmproto.SendReactionRequest_Action
	switch r.GetAction() {
	case pb.ReactionAction_REACTION_ACTION_ADD:
		action = gmproto.SendReactionRequest_ADD
	case pb.ReactionAction_REACTION_ACTION_REMOVE:
		action = gmproto.SendReactionRequest_REMOVE
	case pb.ReactionAction_REACTION_ACTION_SWITCH:
		action = gmproto.SendReactionRequest_SWITCH
	default:
		return nil, badRequest("reaction action required")
	}
	if r.GetMessageId() == "" || r.GetEmoji() == "" {
		return nil, badRequest("message_id and emoji required")
	}
	return &gmproto.SendReactionRequest{
		MessageID:    r.GetMessageId(),
		ReactionData: gmproto.MakeReactionData(r.GetEmoji()),
		Action:       action,
		SIMPayload:   simPayload(r.GetSim()),
	}, nil
}

func Sims(s *gmproto.Settings) *pb.SimsUpdated {
	out := &pb.SimsUpdated{}
	for _, c := range s.GetSIMCards() {
		d := c.GetSIMData()
		out.Sims = append(out.Sims, &pb.SimCard{
			Sim:             &pb.Sim{SimNumber: d.GetSIMPayload().GetSIMNumber(), PayloadKind: d.GetSIMPayload().GetTwo()},
			ParticipantId:   c.GetSIMParticipant().GetID(),
			CarrierName:     d.GetCarrierName(),
			FormattedNumber: d.GetFormattedPhoneNumber(),
			ColorHex:        d.GetColorHex(),
		})
	}
	return out
}
