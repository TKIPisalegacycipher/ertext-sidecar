package server

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/gmext"
	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/pb"
)

func TestCheckRcsAsksEachNumberAndAnswersInOrder(t *testing.T) {
	h, f := connected(t)
	f.setRCS(map[string]*gmext.GetContactRcsGroupStatusResponse{
		"+15550100": {RcsGroupCapable: true, GroupEncryptionCapable: true},
		"+15550101": {},
	})
	h.send(req(2, &pb.CheckRcs{Numbers: []string{"+15550100", " +15550101 ", "+15550102"}}))
	res := h.response(2).GetRcsStatus().GetResults()
	require.Len(t, res, 3)
	require.True(t, res[0].GetChecked())
	require.True(t, res[0].GetRcsGroup())
	require.True(t, res[0].GetEncryptedGroup())
	require.True(t, res[1].GetChecked())
	require.False(t, res[1].GetRcsGroup())
	require.False(t, res[2].GetChecked(), "the phone didn't answer for this one")
	reqs := f.gotRCS()
	require.Len(t, reqs, 3)
	for _, r := range reqs {
		require.Equal(t, int32(7), r.GetNumber().GetType())
		require.Equal(t, r.GetNumber().GetNumber(), r.GetNumber().GetAltNumber())
		require.Nil(t, r.GetSim())
		require.Nil(t, r.GetConversation())
	}
	h.send(req(3, &pb.CheckRcs{}))
	require.Equal(t, pb.ErrorCode_ERROR_CODE_BAD_REQUEST, h.response(3).GetError().GetCode())
	require.NoError(t, h.close())
}
