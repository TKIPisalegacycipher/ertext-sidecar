package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"

	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/pb"
)

func TestListConversations(t *testing.T) {
	h, f := connected(t)
	f.setList(&gmproto.ListConversationsResponse{
		Conversations: []*gmproto.Conversation{{ConversationID: "c1"}},
		Cursor:        &gmproto.Cursor{LastItemID: "c1", LastItemTimestamp: 5},
	})
	h.send(req(2, &pb.ListConversations{Folder: pb.Folder_FOLDER_ARCHIVE}))
	page := h.response(2).GetConversationPage()
	require.Equal(t, "c1", page.GetConversations()[0].GetId())
	require.Equal(t, "c1", page.GetNext().GetLastItemId())
	lr := f.gotList()
	require.EqualValues(t, 100, lr.GetCount())
	require.Equal(t, gmproto.ListConversationsRequest_ARCHIVE, lr.GetFolder())
	require.Nil(t, lr.Cursor)

	h.send(req(3, &pb.ListConversations{Count: 7, Cursor: &pb.Cursor{LastItemId: "c1", LastItemTsMs: 5}}))
	h.response(3)
	lr = f.gotList()
	require.EqualValues(t, 7, lr.GetCount())
	require.Equal(t, "c1", lr.GetCursor().GetLastItemID())
	require.NoError(t, h.close())
}

func TestFetchMessages(t *testing.T) {
	h, f := connected(t)
	f.setFetch(&gmproto.ListMessagesResponse{Messages: []*gmproto.Message{
		{MessageID: "m2", Timestamp: 2_000_000}, {MessageID: "m1", Timestamp: 1_000_000},
	}})
	h.send(req(2, &pb.FetchMessages{ConversationId: "c1"}))
	page := h.response(2).GetMessagePage()
	require.Equal(t, "m1", page.GetMessages()[0].GetId())
	require.Equal(t, "m1", page.GetNext().GetLastItemId())
	require.Equal(t, []any{"c1", int64(25), (*gmproto.Cursor)(nil)}, f.gotFetch())

	h.send(req(3, &pb.FetchMessages{}))
	require.Equal(t, pb.ErrorCode_ERROR_CODE_BAD_REQUEST, h.response(3).GetError().GetCode())
	require.NoError(t, h.close())
}

func TestSendMessage(t *testing.T) {
	h, f := connected(t)
	f.setSend(&gmproto.SendMessageResponse{Status: gmproto.SendMessageResponse_SUCCESS})
	h.send(req(2, &pb.SendMessage{ConversationId: "c1", TmpId: "tmp_1", Text: "hi", OutgoingParticipantId: "p-me"}))
	require.NotNil(t, h.response(2).GetAck())
	require.Equal(t, "tmp_1", f.gotSend().GetTmpID())

	f.setSend(&gmproto.SendMessageResponse{Status: gmproto.SendMessageResponse_FAILURE_2})
	h.send(req(3, &pb.SendMessage{ConversationId: "c1", TmpId: "tmp_2", Text: "hi"}))
	require.Equal(t, pb.ErrorCode_ERROR_CODE_SEND_FAILED, h.response(3).GetError().GetCode())

	h.send(req(4, &pb.SendMessage{ConversationId: "c1", Text: "no tmp id"}))
	require.Equal(t, pb.ErrorCode_ERROR_CODE_BAD_REQUEST, h.response(4).GetError().GetCode())
	require.NoError(t, h.close())
}

func TestSendReaction(t *testing.T) {
	h, f := connected(t)
	f.setReact(&gmproto.SendReactionResponse{Success: true})
	h.send(req(2, &pb.SendReaction{MessageId: "m1", Emoji: "👍", Action: pb.ReactionAction_REACTION_ACTION_ADD}))
	require.NotNil(t, h.response(2).GetAck())
	require.Equal(t, "m1", f.gotReact().GetMessageID())

	f.setReact(&gmproto.SendReactionResponse{Success: false})
	h.send(req(3, &pb.SendReaction{MessageId: "m1", Emoji: "👍", Action: pb.ReactionAction_REACTION_ACTION_REMOVE}))
	require.Equal(t, pb.ErrorCode_ERROR_CODE_SEND_FAILED, h.response(3).GetError().GetCode())
	require.NoError(t, h.close())
}

func TestMarkRead(t *testing.T) {
	h, f := connected(t)
	h.send(req(2, &pb.MarkRead{ConversationId: "c1", MessageId: "m9"}))
	require.NotNil(t, h.response(2).GetAck())
	require.Equal(t, [2]string{"c1", "m9"}, f.gotRead())
	h.send(req(3, &pb.MarkRead{ConversationId: "c1"}))
	require.Equal(t, pb.ErrorCode_ERROR_CODE_BAD_REQUEST, h.response(3).GetError().GetCode())
	require.NoError(t, h.close())
}

func TestThumbnails(t *testing.T) {
	h, f := connected(t)
	f.setThumbs(&gmproto.GetThumbnailResponse{Thumbnail: []*gmproto.GetThumbnailResponse_Thumbnail{
		{Identifier: "p2", Data: &gmproto.ThumbnailData{ImageBuffer: []byte{0x89, 'P', 'N', 'G'}}},
	}})
	h.send(req(2, &pb.GetThumbnails{ParticipantIds: []string{"p2"}}))
	got := h.response(2).GetThumbnails().GetThumbnails()
	require.Len(t, got, 1)
	require.Equal(t, "p2", got[0].GetParticipantId())
	require.Equal(t, []byte{0x89, 'P', 'N', 'G'}, got[0].GetImage())
	require.Equal(t, []string{"p2"}, f.gotThumbIDs())
	require.NoError(t, h.close())
}

func TestDownloadMedia(t *testing.T) {
	h, f := connected(t)
	f.setMedia([]byte("jpeg bytes"))
	dest := filepath.Join(t.TempDir(), "media", "md1.jpg")
	h.send(req(2, &pb.DownloadMedia{MediaId: "md1", Key: []byte{1}, DestPath: dest}))
	require.NotNil(t, h.response(2).GetAck())
	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	require.Equal(t, []byte("jpeg bytes"), got)
	_, err = os.Stat(dest + ".part")
	require.True(t, os.IsNotExist(err))

	h.send(req(3, &pb.DownloadMedia{MediaId: "md1", DestPath: "relative.jpg"}))
	require.Equal(t, pb.ErrorCode_ERROR_CODE_BAD_REQUEST, h.response(3).GetError().GetCode())
	require.NoError(t, h.close())
}
