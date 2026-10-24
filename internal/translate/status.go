// Package translate converts between libgm's gmproto types and Ertext's protocol (pb). It holds
// no state; every function is pure.
package translate

import (
	"go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"

	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/pb"
)

const firstIncoming, firstTombstone = 100, 200

var failed = map[gmproto.MessageStatusType]bool{
	gmproto.MessageStatusType_OUTGOING_FAILED_GENERIC:                                                 true,
	gmproto.MessageStatusType_OUTGOING_FAILED_EMERGENCY_NUMBER:                                        true,
	gmproto.MessageStatusType_OUTGOING_CANCELED:                                                       true,
	gmproto.MessageStatusType_OUTGOING_FAILED_TOO_LARGE:                                               true,
	gmproto.MessageStatusType_OUTGOING_FAILED_RECIPIENT_LOST_RCS:                                      true,
	gmproto.MessageStatusType_OUTGOING_FAILED_NO_RETRY_NO_FALLBACK:                                    true,
	gmproto.MessageStatusType_OUTGOING_FAILED_RECIPIENT_DID_NOT_DECRYPT:                               true,
	gmproto.MessageStatusType_OUTGOING_FAILED_RECIPIENT_LOST_ENCRYPTION:                               true,
	gmproto.MessageStatusType_OUTGOING_FAILED_RECIPIENT_DID_NOT_DECRYPT_NO_MORE_RETRY:                 true,
	gmproto.MessageStatusType_OUTGOING_FAILED_RECIPIENT_NEGATIVE_DELIVERY:                             true,
	gmproto.MessageStatusType_MESSAGE_STATUS_OUTGOING_FAILED_EMERGENCY_PROTOCOL_DETERMINATION_MESSAGE: true,
	gmproto.MessageStatusType_OUTGOING_RESTRICTED:                                                     true,
	gmproto.MessageStatusType_OUTGOING_FAILED_TO_ENCRYPT:                                              true,
}

var downloading = map[gmproto.MessageStatusType]bool{
	gmproto.MessageStatusType_INCOMING_YET_TO_MANUAL_DOWNLOAD:   true,
	gmproto.MessageStatusType_INCOMING_RETRYING_MANUAL_DOWNLOAD: true,
	gmproto.MessageStatusType_INCOMING_MANUAL_DOWNLOADING:       true,
	gmproto.MessageStatusType_INCOMING_RETRYING_AUTO_DOWNLOAD:   true,
	gmproto.MessageStatusType_INCOMING_AUTO_DOWNLOADING:         true,
	gmproto.MessageStatusType_INCOMING_AWAITING_AUTO_DOWNLOAD:   true,
}

// IsDeleted reports the one status that is a true removal (sent as MessageDeleted):
// MESSAGE_DELETED (300), as in mautrix-gmessages. OUTGOING_DELETED (23) and INCOMING_DELETED (117)
// are upserts that Status maps to MESSAGE_STATUS_DELETED.
func IsDeleted(s gmproto.MessageStatusType) bool {
	return s == gmproto.MessageStatusType_MESSAGE_DELETED
}

func isDeletedPlaceholder(s gmproto.MessageStatusType) bool {
	return s == gmproto.MessageStatusType_OUTGOING_DELETED || s == gmproto.MessageStatusType_INCOMING_DELETED
}

func isOutgoing(s gmproto.MessageStatusType) bool {
	return s > 0 && s < firstIncoming
}

func isTombstone(s gmproto.MessageStatusType) bool {
	return s >= firstTombstone
}

func Status(s gmproto.MessageStatusType) pb.MessageStatus {
	switch {
	case s == gmproto.MessageStatusType_STATUS_UNKNOWN:
		return pb.MessageStatus_MESSAGE_STATUS_UNSPECIFIED
	case IsDeleted(s), isDeletedPlaceholder(s):
		return pb.MessageStatus_MESSAGE_STATUS_DELETED
	case s == gmproto.MessageStatusType_OUTGOING_COMPLETE:
		return pb.MessageStatus_MESSAGE_STATUS_SENT
	case s == gmproto.MessageStatusType_OUTGOING_DELIVERED:
		return pb.MessageStatus_MESSAGE_STATUS_DELIVERED
	case s == gmproto.MessageStatusType_OUTGOING_DISPLAYED:
		return pb.MessageStatus_MESSAGE_STATUS_READ
	case failed[s]:
		return pb.MessageStatus_MESSAGE_STATUS_FAILED
	case isOutgoing(s):
		return pb.MessageStatus_MESSAGE_STATUS_PENDING
	case downloading[s]:
		return pb.MessageStatus_MESSAGE_STATUS_INCOMING_DOWNLOADING
	default: // remaining incoming statuses and tombstones
		return pb.MessageStatus_MESSAGE_STATUS_INCOMING
	}
}

// kind maps libgm's Message.type (1 sms, 2 downloaded mms, 3 undownloaded mms, else rcs).
func kind(msgType int64, s gmproto.MessageStatusType) pb.MessageKind {
	if isTombstone(s) {
		return pb.MessageKind_MESSAGE_KIND_SYSTEM
	}
	switch msgType {
	case 1:
		return pb.MessageKind_MESSAGE_KIND_SMS
	case 2, 3:
		return pb.MessageKind_MESSAGE_KIND_MMS
	default:
		return pb.MessageKind_MESSAGE_KIND_RCS
	}
}

func convType(t gmproto.ConversationType) pb.ConversationType {
	switch t {
	case gmproto.ConversationType_SMS:
		return pb.ConversationType_CONVERSATION_TYPE_SMS
	case gmproto.ConversationType_RCS:
		return pb.ConversationType_CONVERSATION_TYPE_RCS
	default:
		return pb.ConversationType_CONVERSATION_TYPE_UNSPECIFIED
	}
}

func convStatus(s gmproto.ConversationStatus) pb.ConversationStatus {
	switch s {
	case gmproto.ConversationStatus_UNKNOWN_CONVERSATION_STATUS:
		return pb.ConversationStatus_CONVERSATION_STATUS_UNSPECIFIED
	case gmproto.ConversationStatus_ACTIVE:
		return pb.ConversationStatus_CONVERSATION_STATUS_ACTIVE
	case gmproto.ConversationStatus_ARCHIVED, gmproto.ConversationStatus_KEEP_ARCHIVED:
		return pb.ConversationStatus_CONVERSATION_STATUS_ARCHIVED
	case gmproto.ConversationStatus_DELETED, gmproto.ConversationStatus_TRASH_FOLDER:
		return pb.ConversationStatus_CONVERSATION_STATUS_DELETED
	case gmproto.ConversationStatus_SPAM_FOLDER:
		return pb.ConversationStatus_CONVERSATION_STATUS_SPAM
	case gmproto.ConversationStatus_BLOCKED_FOLDER:
		return pb.ConversationStatus_CONVERSATION_STATUS_BLOCKED
	default:
		return pb.ConversationStatus_CONVERSATION_STATUS_OTHER
	}
}
