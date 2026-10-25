package gm

import (
	"context"
	"fmt"
	_ "unsafe" // go:linkname

	"go.mau.fi/mautrix-gmessages/pkg/libgm"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"
	"google.golang.org/protobuf/proto"

	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/gmext"
)

// libgm only sends the actions it has a method for, and its sender is unexported. These two
// pull-linknames reach it without editing libgm. They are tied to libgm's internals at the
// version in go.mod: TestLinknamesReachLibgm fails if a bump renames them.

//go:linkname sessionHandlerOf go.mau.fi/mautrix-gmessages/pkg/libgm.(*Client).getSessionHandler
func sessionHandlerOf(c *libgm.Client) *libgm.SessionHandler

//go:linkname sendWithParams go.mau.fi/mautrix-gmessages/pkg/libgm.(*SessionHandler).sendMessageWithParams
func sendWithParams(s *libgm.SessionHandler, ctx context.Context, params libgm.SendMessageParams) (*libgm.IncomingRPCMessage, error)

// GetContactRCSGroupStatus asks the phone whether a number can be in an RCS group (ActionType 26,
// a read-only query in Messages for Web). Reverse-engineered and unverified on a real phone.
func (c *libgmClient) GetContactRCSGroupStatus(ctx context.Context, req *gmext.GetContactRcsGroupStatusRequest) (*gmext.GetContactRcsGroupStatusResponse, error) {
	resp, err := sendWithParams(sessionHandlerOf(c.Client), ctx, libgm.SendMessageParams{
		Action: gmproto.ActionType_GET_CONTACT_RCS_GROUP_STATUS,
		Data:   req,
	})
	if err != nil {
		return nil, err
	}
	return decodeRCSStatus(resp.DecryptedData)
}

// decodeRCSStatus decodes the answer's decrypted bytes: libgm has no response type for action 26
// (its responseType map), so it leaves them undecoded. No bytes means both fields false.
func decodeRCSStatus(data []byte) (*gmext.GetContactRcsGroupStatusResponse, error) {
	out := &gmext.GetContactRcsGroupStatusResponse{}
	if err := proto.Unmarshal(data, out); err != nil {
		return nil, fmt.Errorf("decode RCS status: %w", err)
	}
	return out, nil
}
