package translate_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"
	"google.golang.org/protobuf/proto"

	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/pb"
	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/translate"
)

func TestMessageCarriesEmbeddedMediaBytes(t *testing.T) {
	in := gmMsg("m1", 1, gmproto.MessageStatusType_INCOMING_COMPLETE)
	in.MessageInfo = []*gmproto.MessageInfo{{Data: &gmproto.MessageInfo_MediaContent{MediaContent: &gmproto.MediaContent{
		MimeType: "image/jpeg", MediaName: "e.jpg", Size: 3, MediaData: []byte{0xff, 0xd8, 0xff},
		Dimensions: &gmproto.Dimensions{Width: 2, Height: 1},
	}}}}
	got := translate.Message(in)
	require.Equal(t, []*pb.Media{{
		Mime: "image/jpeg", Name: "e.jpg", Size: 3, Width: 2, Height: 1, Data: []byte{0xff, 0xd8, 0xff},
	}}, got.Media)
}

func TestMessageHandlesLocationContent(t *testing.T) {
	in := gmMsg("m1", 1, gmproto.MessageStatusType_INCOMING_COMPLETE)
	in.MessageInfo = []*gmproto.MessageInfo{
		{Data: &gmproto.MessageInfo_MessageContent{MessageContent: &gmproto.MessageContent{Content: "before"}}},
		{Data: &gmproto.MessageInfo_LocationContent{LocationContent: &gmproto.LocationContent{
			MessageContent: &gmproto.MessageContent{Content: "Pinned place"},
			MediaContent:   &gmproto.MediaContent{MediaID: "map1", MimeType: "image/png", MediaData: []byte{1}},
		}}},
		{ActionMessageID: proto.String("a"), Data: &gmproto.MessageInfo_LocationContent{LocationContent: &gmproto.LocationContent{}}},
	}
	got := translate.Message(in)
	require.Equal(t, "before\nPinned place", got.Text)
	require.Len(t, got.Media, 1)
	require.Equal(t, "map1", got.Media[0].MediaId)
	require.Equal(t, []byte{1}, got.Media[0].Data)
}

func TestMediaStatsCountsTypesAndEmbeddedBytesWithoutContent(t *testing.T) {
	in := gmMsg("m1", 1, gmproto.MessageStatusType_INCOMING_COMPLETE)
	in.MessageInfo = []*gmproto.MessageInfo{
		{Data: &gmproto.MessageInfo_MessageContent{MessageContent: &gmproto.MessageContent{Content: "secret"}}},
		{Data: &gmproto.MessageInfo_MediaContent{MediaContent: &gmproto.MediaContent{MediaID: "x"}}},
		{Data: &gmproto.MessageInfo_MediaContent{MediaContent: &gmproto.MediaContent{MediaData: []byte{1, 2, 3}}}},
		{Data: &gmproto.MessageInfo_LocationContent{LocationContent: &gmproto.LocationContent{}}},
		{},
	}
	require.Equal(t, translate.Stats{
		Text: 1, Media: 2, Location: 1, Other: 1, EmptyMediaID: 1, EmbeddedBytes: 3,
	}, translate.MediaStats(in))
}
