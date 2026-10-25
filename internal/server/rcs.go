package server

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/gm"
	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/gmext"
	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/pb"
)

// rcsCheckTimeout bounds a whole CheckRcs: every number is asked at once.
const rcsCheckTimeout = 10 * time.Second

// numberTyped is the ContactNumber type Messages for Web uses for a typed destination.
const numberTyped int32 = 7

// checkRcs asks the phone, for every number at once, whether it can be in an RCS group. A number
// whose question fails, times out or panics comes back unchecked. Logs counts only.
func (s *Server) checkRcs(r *pb.CheckRcs) *pb.Response {
	numbers, err := cleanNumbers(r.GetNumbers())
	if err != nil {
		return errResp(err)
	}
	return s.withClient(rcsCheckTimeout, func(ctx context.Context, c gm.Client) *pb.Response {
		results := make([]*pb.RcsCapability, len(numbers))
		var wg sync.WaitGroup
		for i, n := range numbers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results[i] = &pb.RcsCapability{}
				defer func() {
					if p := recover(); p != nil {
						s.log.Error().Str("panic_type", fmt.Sprintf("%T", p)).Msg("rcs check panicked")
					}
				}()
				results[i] = rcsCapability(c.GetContactRCSGroupStatus(ctx, &gmext.GetContactRcsGroupStatusRequest{
					Number: &gmext.ContactNumber{Type: numberTyped, Number: n, AltNumber: n},
				}))
			}()
		}
		wg.Wait()
		checked := 0
		for _, res := range results {
			if res.GetChecked() {
				checked++
			}
		}
		s.log.Info().Int("numbers", len(numbers)).Int("checked", checked).Msg("rcs check")
		return &pb.Response{Result: &pb.Response_RcsStatus{RcsStatus: &pb.RcsStatus{Results: results}}}
	})
}

func rcsCapability(resp *gmext.GetContactRcsGroupStatusResponse, err error) *pb.RcsCapability {
	if err != nil || resp == nil {
		return &pb.RcsCapability{}
	}
	return &pb.RcsCapability{
		Checked:        true,
		RcsGroup:       resp.GetRcsGroupCapable(),
		EncryptedGroup: resp.GetGroupEncryptionCapable(),
	}
}
