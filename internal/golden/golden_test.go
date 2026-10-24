// Package golden pins the wire bytes of representative frames. Rust (Ertext's ertext-proto crate)
// decodes the same files, so both sides agree on the schema. Regenerate with:
//
//	go test ./internal/golden -update
package golden

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/pb"
)

var update = flag.Bool("update", false, "rewrite proto/testdata/*.bin")

func golden() map[string]*pb.Envelope {
	return map[string]*pb.Envelope{
		"hello": {Kind: &pb.Envelope_Event{Event: &pb.Event{Body: &pb.Event_Hello{Hello: &pb.Hello{
			ProtocolVersion: 1, SidecarVersion: "golden", LibgmVersion: "v0.0.0-golden",
		}}}}},
		"send_message": {Kind: &pb.Envelope_Request{Request: &pb.Request{Id: 7, Body: &pb.Request_SendMessage{SendMessage: &pb.SendMessage{
			ConversationId: "c1", OutgoingParticipantId: "p-me", Sim: &pb.Sim{SimNumber: 1, PayloadKind: 1},
			Text: "héllo 👋", ReplyToId: "m0", TmpId: "tmp_1",
		}}}}},
		"message_upserted": {Kind: &pb.Envelope_Event{Event: &pb.Event{Body: &pb.Event_MessageUpserted{MessageUpserted: &pb.MessageUpserted{
			Message: &pb.Message{
				Id: "m1", TmpId: "tmp_1", ConversationId: "c1", SenderParticipantId: "p-me",
				TimestampUs: 1761300000000000, Kind: pb.MessageKind_MESSAGE_KIND_RCS,
				Status: pb.MessageStatus_MESSAGE_STATUS_DELIVERED, RawStatus: 2,
				Text: "héllo 👋", ReplyToId: "m0", FromMe: true,
				Media: []*pb.Media{{
					MediaId: "md1", Key: []byte{1, 2}, ThumbMediaId: "md1t", ThumbKey: []byte{3},
					Mime: "image/jpeg", Name: "pic.jpg", Size: 1234, Width: 640, Height: 480,
				}},
				Reactions: []*pb.Reaction{{Emoji: "👍", ParticipantIds: []string{"p2"}}},
			},
		}}}}},
		"error_response": {Kind: &pb.Envelope_Response{Response: &pb.Response{Id: 9, Result: &pb.Response_Error{Error: &pb.RpcError{
			Code: pb.ErrorCode_ERROR_CODE_UNAUTHORIZED, Message: "session invalid",
		}}}}},
	}
}

func TestGolden(t *testing.T) {
	dir := filepath.Join("..", "..", "proto", "testdata")
	for name, env := range golden() {
		b, err := proto.MarshalOptions{Deterministic: true}.Marshal(env)
		require.NoError(t, err)
		path := filepath.Join(dir, name+".bin")
		if *update {
			require.NoError(t, os.WriteFile(path, b, 0o644))
			continue
		}
		want, err := os.ReadFile(path)
		require.NoError(t, err, "missing golden %s: run go test ./internal/golden -update", path)
		require.Equal(t, want, b, "golden %s changed: the wire format moved", name)
		var back pb.Envelope
		require.NoError(t, proto.Unmarshal(want, &back))
		require.True(t, proto.Equal(env, &back), name)
	}
}
