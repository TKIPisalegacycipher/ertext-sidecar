package translate

import (
	"errors"
	"net/http"

	"go.mau.fi/mautrix-gmessages/pkg/libgm"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/events"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"

	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/pb"
)

// IsCredentialError mirrors mautrix-gmessages' invalidateSessionIfCredentialError: these mean the
// pairing is gone and the user must pair again.
func IsCredentialError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, events.ErrInvalidCredentials) || errors.Is(err, events.ErrRequestedEntityNotFound) {
		return true
	}
	var httpErr events.HTTPError
	return errors.As(err, &httpErr) && httpErr.Action == "polling" &&
		httpErr.Resp != nil && httpErr.Resp.StatusCode == http.StatusUnauthorized
}

// errText is err.Error(), or "" for a nil error (libgm events can carry one).
func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// MobileDBSync reports whether evt is the phone's "database sync started/complete" alert, and if
// so whether the phone is now syncing. While it syncs, mautrix-gmessages ignores MESSAGE_DELETED
// (see SuppressedDuringDBSync); the server keeps that flag per client.
func MobileDBSync(evt any) (syncing, ok bool) {
	a, isAlert := evt.(*gmproto.UserAlertEvent)
	if !isAlert {
		return false, false
	}
	switch a.GetAlertType() {
	case gmproto.AlertType_MOBILE_DATABASE_SYNC_STARTED:
		return true, true
	case gmproto.AlertType_MOBILE_DATABASE_SYNC_COMPLETE:
		return false, true
	}
	return false, false
}

// SuppressedDuringDBSync reports events mautrix-gmessages drops while the phone's database syncs:
// MESSAGE_DELETED (300) messages, which the phone emits spuriously during a resync.
func SuppressedDuringDBSync(evt any) bool {
	m, ok := evt.(*libgm.WrappedMessage)
	return ok && m != nil && m.Message != nil &&
		m.GetMessageStatus().GetStatus() == gmproto.MessageStatusType_MESSAGE_DELETED
}

func resyncNeeded() []*pb.Event {
	return []*pb.Event{{Body: &pb.Event_ResyncNeeded{ResyncNeeded: &pb.ResyncNeeded{}}}}
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
		if e == nil || e.Message == nil {
			return nil
		}
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
		case gmproto.AlertType_MOBILE_DATABASE_SYNC_COMPLETE:
			return resyncNeeded() // mautrix-gmessages runs a minimal conversation sync here
		}
		return nil
	case *events.PhoneNotResponding:
		return state(pb.ConnState_CONN_STATE_PHONE_OFFLINE, "")
	case *events.PhoneRespondingAgain:
		return state(pb.ConnState_CONN_STATE_PHONE_ONLINE, "")
	case *events.ListenTemporaryError:
		return state(pb.ConnState_CONN_STATE_NETWORK_DOWN, errText(e.Error))
	case *events.ListenRecovered:
		return state(pb.ConnState_CONN_STATE_CONNECTED, "")
	case *events.ListenFatalError:
		if IsCredentialError(e.Error) {
			return state(pb.ConnState_CONN_STATE_UNPAIRED, errText(e.Error))
		}
		return state(pb.ConnState_CONN_STATE_ERROR, errText(e.Error))
	case *gmproto.RevokePairData:
		return state(pb.ConnState_CONN_STATE_UNPAIRED, "pairing revoked on phone")
	case *events.GaiaLoggedOut:
		return state(pb.ConnState_CONN_STATE_UNPAIRED, "google account logged out")
	case *events.PingFailed:
		if errors.Is(e.Error, events.ErrRequestedEntityNotFound) {
			return state(pb.ConnState_CONN_STATE_UNPAIRED, errText(e.Error))
		}
		return nil
	case *events.NoDataReceived:
		return resyncNeeded()
	}
	return nil
}
