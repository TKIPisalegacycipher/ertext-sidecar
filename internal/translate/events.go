package translate

import (
	"errors"

	"go.mau.fi/mautrix-gmessages/pkg/libgm"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/events"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"

	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/pb"
)

// IsCredentialError mirrors mautrix-gmessages' invalidateSessionIfCredentialError: these mean the
// pairing is gone and the user must pair again.
func IsCredentialError(err error) bool {
	return err != nil &&
		(errors.Is(err, events.ErrInvalidCredentials) || errors.Is(err, events.ErrRequestedEntityNotFound))
}

func state(s pb.ConnState, detail string) []*pb.Event {
	return []*pb.Event{{Body: &pb.Event_ConnectionState{ConnectionState: &pb.ConnectionState{State: s, Detail: detail}}}}
}

func convUpserted(c *gmproto.Conversation) *pb.Event {
	return &pb.Event{Body: &pb.Event_ConversationUpserted{ConversationUpserted: &pb.ConversationUpserted{Conversation: Conversation(c)}}}
}

// Event converts one libgm event into zero or more protocol events. Anything Ertext doesn't use
// (typing, battery and other alerts, account changes...) returns nil.
func Event(evt any) []*pb.Event {
	switch e := evt.(type) {
	case *events.ClientReady:
		out := make([]*pb.Event, 0, len(e.Conversations))
		for _, c := range e.Conversations {
			out = append(out, convUpserted(c))
		}
		return out
	case *gmproto.Conversation:
		return []*pb.Event{convUpserted(e)}
	case *libgm.WrappedMessage:
		if IsDeleted(e.GetMessageStatus().GetStatus()) {
			return []*pb.Event{{Body: &pb.Event_MessageDeleted{MessageDeleted: &pb.MessageDeleted{
				ConversationId: e.GetConversationID(), MessageId: e.GetMessageID(),
			}}}}
		}
		return []*pb.Event{{Body: &pb.Event_MessageUpserted{MessageUpserted: &pb.MessageUpserted{
			Message: Message(e.Message), IsOld: e.IsOld,
		}}}}
	case *gmproto.Settings:
		return []*pb.Event{{Body: &pb.Event_SimsUpdated{SimsUpdated: Sims(e)}}}
	case *gmproto.UserAlertEvent:
		switch e.GetAlertType() {
		case gmproto.AlertType_BROWSER_ACTIVE:
			return state(pb.ConnState_CONN_STATE_CONNECTED, "")
		case gmproto.AlertType_BROWSER_INACTIVE,
			gmproto.AlertType_BROWSER_INACTIVE_FROM_TIMEOUT,
			gmproto.AlertType_BROWSER_INACTIVE_FROM_INACTIVITY:
			return state(pb.ConnState_CONN_STATE_TAKEN_ELSEWHERE, e.GetAlertType().String())
		}
		return nil
	case *events.PhoneNotResponding:
		return state(pb.ConnState_CONN_STATE_PHONE_OFFLINE, "")
	case *events.PhoneRespondingAgain:
		return state(pb.ConnState_CONN_STATE_PHONE_ONLINE, "")
	case *events.ListenTemporaryError:
		return state(pb.ConnState_CONN_STATE_NETWORK_DOWN, e.Error.Error())
	case *events.ListenRecovered:
		return state(pb.ConnState_CONN_STATE_CONNECTED, "")
	case *events.ListenFatalError:
		if IsCredentialError(e.Error) {
			return state(pb.ConnState_CONN_STATE_UNPAIRED, e.Error.Error())
		}
		return state(pb.ConnState_CONN_STATE_ERROR, e.Error.Error())
	case *gmproto.RevokePairData:
		return state(pb.ConnState_CONN_STATE_UNPAIRED, "pairing revoked on phone")
	case *events.GaiaLoggedOut:
		return state(pb.ConnState_CONN_STATE_UNPAIRED, "google account logged out")
	case *events.PingFailed:
		if errors.Is(e.Error, events.ErrRequestedEntityNotFound) {
			return state(pb.ConnState_CONN_STATE_UNPAIRED, e.Error.Error())
		}
		return nil
	case *events.NoDataReceived:
		return []*pb.Event{{Body: &pb.Event_ResyncNeeded{ResyncNeeded: &pb.ResyncNeeded{}}}}
	}
	return nil
}
