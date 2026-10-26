package server

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"
	"go.mau.fi/util/ptr"

	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/pb"
)

func gmConversation(id string) *gmproto.Conversation {
	return &gmproto.Conversation{ConversationID: id, Type: gmproto.ConversationType_RCS}
}

func TestGetOrCreateConversationOneToOne(t *testing.T) {
	h, f := connected(t)
	f.setCreate(&gmproto.GetOrCreateConversationResponse{
		Status: gmproto.GetOrCreateConversationResponse_SUCCESS, Conversation: gmConversation("c9"),
	})
	h.send(req(2, &pb.GetOrCreateConversation{Numbers: []string{" +15550100 "}, AllowRcsGroup: true}))
	got := h.response(2).GetConversationCreated().GetConversation()
	require.Equal(t, "c9", got.GetId())
	require.Equal(t, pb.ConversationType_CONVERSATION_TYPE_RCS, got.GetType())
	reqs := f.gotCreate()
	require.Len(t, reqs, 1)
	n := reqs[0].GetNumbers()
	require.Len(t, n, 1)
	require.Equal(t, int32(2), n[0].GetMysteriousInt())
	require.Equal(t, "+15550100", n[0].GetNumber(), "trimmed")
	require.Equal(t, "+15550100", n[0].GetNumber2())
	require.Nil(t, reqs[0].CreateRCSGroup, "the phone decides")
	require.NoError(t, h.close())
}

func TestGetOrCreateConversationGroupTakesTheRCSOffer(t *testing.T) {
	h, f := connected(t)
	f.setCreate(
		&gmproto.GetOrCreateConversationResponse{Status: gmproto.GetOrCreateConversationResponse_CREATE_RCS},
		&gmproto.GetOrCreateConversationResponse{
			Status: gmproto.GetOrCreateConversationResponse_SUCCESS, Conversation: gmConversation("g1"),
		},
	)
	h.send(req(2, &pb.GetOrCreateConversation{Numbers: []string{"+15550100", "+15550101"}, AllowRcsGroup: true}))
	require.Equal(t, "g1", h.response(2).GetConversationCreated().GetConversation().GetId())
	reqs := f.gotCreate()
	require.Len(t, reqs, 2, "CREATE_RCS: asked again")
	require.Nil(t, reqs[0].CreateRCSGroup)
	require.True(t, reqs[1].GetCreateRCSGroup())
	require.NotNil(t, reqs[1].RCSGroupName)
	require.Equal(t, "", reqs[1].GetRCSGroupName())
	require.Len(t, reqs[1].GetNumbers(), 2)
	require.NoError(t, h.close())
}

func TestGetOrCreateConversationWithoutAnIDFails(t *testing.T) {
	h, f := connected(t)
	f.setCreate(&gmproto.GetOrCreateConversationResponse{Status: gmproto.GetOrCreateConversationResponse_CREATE_RCS})
	h.send(req(2, &pb.GetOrCreateConversation{Numbers: []string{"+15550100", "+15550101"}}))
	e := h.response(2).GetError()
	require.Equal(t, pb.ErrorCode_ERROR_CODE_SEND_FAILED, e.GetCode())
	require.Len(t, f.gotCreate(), 1, "allow_rcs_group unset: the offer isn't taken")
	require.NotContains(t, e.GetMessage(), "5550100", "errors never quote a number")
	require.NoError(t, h.close())
}

func TestGetOrCreateConversationRefusesBadNumbers(t *testing.T) {
	h, f := connected(t)
	tooMany := make([]string, maxRecipients+1)
	for i := range tooMany {
		tooMany[i] = "+15550100"
	}
	for i, numbers := range [][]string{nil, {"+15550100", "  "}, tooMany} {
		id := uint64(10 + i)
		h.send(req(id, &pb.GetOrCreateConversation{Numbers: numbers}))
		e := h.response(id).GetError()
		require.Equal(t, pb.ErrorCode_ERROR_CODE_BAD_REQUEST, e.GetCode())
		require.NotContains(t, e.GetMessage(), "5550100")
	}
	require.Empty(t, f.gotCreate())
	require.NoError(t, h.close())
}

func TestListContacts(t *testing.T) {
	h, f := connected(t)
	f.setContacts(
		&gmproto.ListContactsResponse{Contacts: []*gmproto.Contact{
			{ContactID: "k1", ParticipantID: "p7", Name: "Ann", Number: &gmproto.ContactNumber{Number: "+15550100", FormattedNumber: ptr.Ptr("+1 555-0100")}},
			{ContactID: "k2", Name: "No number"},
		}},
		&gmproto.ListTopContactsResponse{Contacts: []*gmproto.Contact{
			{ContactID: "k3", Name: "Top", Number: &gmproto.ContactNumber{Number: "+15550103"}},
		}},
	)
	h.send(req(2, &pb.ListContacts{}))
	all := h.response(2).GetContacts().GetContacts()
	require.Len(t, all, 1, "a contact without a number is dropped")
	require.Equal(t, "k1", all[0].GetContactId())
	require.Equal(t, "Ann", all[0].GetName())
	require.Equal(t, "+15550100", all[0].GetNumber())
	require.Equal(t, "+1 555-0100", all[0].GetFormattedNumber())
	require.Equal(t, "p7", all[0].GetParticipantId())
	h.send(req(3, &pb.ListContacts{Top: true}))
	top := h.response(3).GetContacts().GetContacts()
	require.Len(t, top, 1)
	require.Equal(t, "Top", top[0].GetName())
	require.Empty(t, top[0].GetParticipantId(), "no participant on the phone's contact")
	require.Contains(t, f.Calls(), "contacts")
	require.Contains(t, f.Calls(), "topcontacts")
	require.NoError(t, h.close())
}
