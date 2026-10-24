package pb_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/pb"
)

func TestEnvelopeRoundTrip(t *testing.T) {
	in := &pb.Envelope{Kind: &pb.Envelope_Request{Request: &pb.Request{
		Id: 42,
		Body: &pb.Request_SendMessage{SendMessage: &pb.SendMessage{
			ConversationId: "c1", TmpId: "tmp_1", Text: "hi",
			Sim: &pb.Sim{SimNumber: 1, PayloadKind: 1},
		}},
	}}}
	b, err := proto.Marshal(in)
	require.NoError(t, err)
	var out pb.Envelope
	require.NoError(t, proto.Unmarshal(b, &out))
	require.True(t, proto.Equal(in, &out))
	require.Equal(t, "tmp_1", out.GetRequest().GetSendMessage().GetTmpId())
}
