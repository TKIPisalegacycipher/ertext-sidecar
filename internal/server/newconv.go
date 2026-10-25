package server

import (
	"context"
	"fmt"
	"strings"

	"go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"
	"go.mau.fi/util/ptr"

	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/gm"
	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/pb"
	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/translate"
)

// maxRecipients bounds a new conversation's numbers (and an RCS check's).
const maxRecipients = 20

// numberPicked is libgm's ContactNumber.MysteriousInt as mautrix-gmessages sends it to create
// conversations ("might be 2 for contact and 7 for user input": reverse-engineered).
const numberPicked int32 = 2

// cleanNumbers trims each number and refuses no numbers, a blank one or too many. Its errors
// never quote a number: Rust logs RpcError messages.
func cleanNumbers(in []string) ([]string, error) {
	if len(in) == 0 {
		return nil, badRequest("numbers required")
	}
	if len(in) > maxRecipients {
		return nil, badRequest("at most %d numbers", maxRecipients)
	}
	out := make([]string, 0, len(in))
	for _, n := range in {
		n = strings.TrimSpace(n)
		if n == "" {
			return nil, badRequest("blank number")
		}
		out = append(out, n)
	}
	return out, nil
}

// getOrCreateConversation finds or creates the conversation with r's numbers. For a group, when
// the phone answers CREATE_RCS (it offers an RCS group) and allow_rcs_group is set, it asks again
// with createRCSGroup, as mautrix-gmessages' CreateGroup does. Logs counts, the status and the id
// only, never a number or the conversation (its proto carries names and numbers).
func (s *Server) getOrCreateConversation(r *pb.GetOrCreateConversation) *pb.Response {
	numbers, err := cleanNumbers(r.GetNumbers())
	if err != nil {
		return errResp(err)
	}
	req := &gmproto.GetOrCreateConversationRequest{}
	for _, n := range numbers {
		req.Numbers = append(req.Numbers, &gmproto.ContactNumber{MysteriousInt: numberPicked, Number: n, Number2: n})
	}
	return s.withClient(requestTimeout, func(ctx context.Context, c gm.Client) *pb.Response {
		resp, err := c.GetOrCreateConversation(ctx, req)
		if err != nil {
			return errResp(err)
		}
		rcsGroup := false
		if resp.GetStatus() == gmproto.GetOrCreateConversationResponse_CREATE_RCS && len(numbers) > 1 && r.GetAllowRcsGroup() {
			rcsGroup = true
			req.RCSGroupName = ptr.Ptr("")
			req.CreateRCSGroup = ptr.Ptr(true)
			resp, err = c.GetOrCreateConversation(ctx, req)
			if err != nil {
				return errResp(err)
			}
		}
		id := resp.GetConversation().GetConversationID()
		s.log.Info().Int("numbers", len(numbers)).Str("status", resp.GetStatus().String()).
			Bool("rcs_group", rcsGroup).Str("conversation_id", id).Msg("get or create conversation")
		if id == "" {
			return errResp(fmt.Errorf("%w: no conversation (status %s)", errSendFailed, resp.GetStatus()))
		}
		return &pb.Response{Result: &pb.Response_ConversationCreated{ConversationCreated: &pb.ConversationCreated{
			Conversation: translate.Conversation(resp.GetConversation()),
		}}}
	})
}

// listContacts lists the phone's contacts (or its top ones). Logs the count only.
func (s *Server) listContacts(r *pb.ListContacts) *pb.Response {
	return s.withClient(requestTimeout, func(ctx context.Context, c gm.Client) *pb.Response {
		var contacts []*gmproto.Contact
		if r.GetTop() {
			resp, err := c.ListTopContacts(ctx)
			if err != nil {
				return errResp(err)
			}
			contacts = resp.GetContacts()
		} else {
			resp, err := c.ListContacts(ctx)
			if err != nil {
				return errResp(err)
			}
			contacts = resp.GetContacts()
		}
		out := translate.Contacts(contacts)
		s.log.Info().Bool("top", r.GetTop()).Int("contacts", len(out.GetContacts())).Msg("listed contacts")
		return &pb.Response{Result: &pb.Response_Contacts{Contacts: out}}
	})
}
