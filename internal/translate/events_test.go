package translate_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mau.fi/mautrix-gmessages/pkg/libgm"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/events"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"

	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/pb"
	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/translate"
)

func connState(t *testing.T, evts []*pb.Event) pb.ConnState {
	t.Helper()
	require.Len(t, evts, 1)
	require.NotNil(t, evts[0].GetConnectionState(), "want ConnectionState, got %v", evts[0])
	return evts[0].GetConnectionState().GetState()
}

func TestEventConnectionStates(t *testing.T) {
	alert := func(a gmproto.AlertType) any { return &gmproto.UserAlertEvent{AlertType: a} }
	cases := []struct {
		in   any
		want pb.ConnState
	}{
		{alert(gmproto.AlertType_BROWSER_ACTIVE), pb.ConnState_CONN_STATE_CONNECTED},
		{alert(gmproto.AlertType_BROWSER_INACTIVE), pb.ConnState_CONN_STATE_TAKEN_ELSEWHERE},
		{alert(gmproto.AlertType_BROWSER_INACTIVE_FROM_TIMEOUT), pb.ConnState_CONN_STATE_TAKEN_ELSEWHERE},
		{alert(gmproto.AlertType_BROWSER_INACTIVE_FROM_INACTIVITY), pb.ConnState_CONN_STATE_TAKEN_ELSEWHERE},
		{&events.PhoneNotResponding{}, pb.ConnState_CONN_STATE_PHONE_OFFLINE},
		{&events.PhoneRespondingAgain{}, pb.ConnState_CONN_STATE_PHONE_ONLINE},
		{&events.ListenTemporaryError{Error: errors.New("dial")}, pb.ConnState_CONN_STATE_NETWORK_DOWN},
		{&events.ListenRecovered{}, pb.ConnState_CONN_STATE_CONNECTED},
		{&events.ListenFatalError{Error: errors.New("boom")}, pb.ConnState_CONN_STATE_ERROR},
		{&events.ListenFatalError{Error: fmt.Errorf("x: %w", events.ErrInvalidCredentials)}, pb.ConnState_CONN_STATE_UNPAIRED},
		{&gmproto.RevokePairData{}, pb.ConnState_CONN_STATE_UNPAIRED},
		{&events.GaiaLoggedOut{}, pb.ConnState_CONN_STATE_UNPAIRED},
		{&events.PingFailed{Error: events.ErrRequestedEntityNotFound, ErrorCount: 1}, pb.ConnState_CONN_STATE_UNPAIRED},
	}
	for _, c := range cases {
		require.Equal(t, c.want, connState(t, translate.Event(c.in)), "%T", c.in)
	}
}

func TestEventIgnored(t *testing.T) {
	require.Nil(t, translate.Event(&events.PingFailed{Error: errors.New("timeout"), ErrorCount: 3}))
	require.Nil(t, translate.Event(&gmproto.UserAlertEvent{AlertType: gmproto.AlertType_MOBILE_BATTERY_LOW}))
	require.Nil(t, translate.Event(&gmproto.TypingData{}))
	require.Nil(t, translate.Event("something unknown"))
}

func TestEventConversationsAndMessages(t *testing.T) {
	ready := translate.Event(&events.ClientReady{Conversations: []*gmproto.Conversation{
		{ConversationID: "c1"}, {ConversationID: "c2"},
	}})
	require.Len(t, ready, 2)
	require.Equal(t, "c2", ready[1].GetConversationUpserted().GetConversation().GetId())

	conv := translate.Event(&gmproto.Conversation{ConversationID: "c3"})
	require.Equal(t, "c3", conv[0].GetConversationUpserted().GetConversation().GetId())

	msg := translate.Event(&libgm.WrappedMessage{IsOld: true, Message: &gmproto.Message{
		MessageID: "m1", ConversationID: "c1",
		MessageStatus: &gmproto.MessageStatus{Status: gmproto.MessageStatusType_INCOMING_COMPLETE},
	}})
	require.Equal(t, "m1", msg[0].GetMessageUpserted().GetMessage().GetId())
	require.True(t, msg[0].GetMessageUpserted().GetIsOld())

	del := translate.Event(&libgm.WrappedMessage{Message: &gmproto.Message{
		MessageID: "m2", ConversationID: "c1",
		MessageStatus: &gmproto.MessageStatus{Status: gmproto.MessageStatusType_INCOMING_DELETED},
	}})
	require.Equal(t, &pb.MessageDeleted{ConversationId: "c1", MessageId: "m2"}, del[0].GetMessageDeleted())
}

func TestEventSimsAndResync(t *testing.T) {
	sims := translate.Event(&gmproto.Settings{SIMCards: []*gmproto.SIMCard{{SIMParticipant: &gmproto.SIMParticipant{ID: "p-me"}}}})
	require.Equal(t, "p-me", sims[0].GetSimsUpdated().GetSims()[0].GetParticipantId())
	resync := translate.Event(&events.NoDataReceived{})
	require.NotNil(t, resync[0].GetResyncNeeded())
}

func TestIsCredentialError(t *testing.T) {
	require.True(t, translate.IsCredentialError(fmt.Errorf("wrap: %w", events.ErrInvalidCredentials)))
	require.True(t, translate.IsCredentialError(events.ErrRequestedEntityNotFound))
	require.False(t, translate.IsCredentialError(errors.New("other")))
	require.False(t, translate.IsCredentialError(nil))
}
