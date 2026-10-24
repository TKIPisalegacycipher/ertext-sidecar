package server

import (
	"context"
	"errors"

	"go.mau.fi/mautrix-gmessages/pkg/libgm"

	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/pb"
	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/translate"
)

var errNotConnected = errors.New("not connected: send Connect or StartPairing first")

// errSendFailed wraps a non-success status from the phone.
var errSendFailed = errors.New("send failed")

func codeFor(err error) pb.ErrorCode {
	switch {
	case errors.Is(err, errNotConnected):
		return pb.ErrorCode_ERROR_CODE_NOT_CONNECTED
	case errors.Is(err, translate.ErrBadRequest):
		return pb.ErrorCode_ERROR_CODE_BAD_REQUEST
	case translate.IsCredentialError(err):
		return pb.ErrorCode_ERROR_CODE_UNAUTHORIZED
	case errors.Is(err, libgm.ErrPhoneNotResponding):
		return pb.ErrorCode_ERROR_CODE_PHONE_NOT_RESPONDING
	case errors.Is(err, errSendFailed):
		return pb.ErrorCode_ERROR_CODE_SEND_FAILED
	case errors.Is(err, context.DeadlineExceeded):
		return pb.ErrorCode_ERROR_CODE_TIMEOUT
	default:
		return pb.ErrorCode_ERROR_CODE_INTERNAL
	}
}

func errResp(err error) *pb.Response {
	return &pb.Response{Result: &pb.Response_Error{Error: &pb.RpcError{Code: codeFor(err), Message: err.Error()}}}
}

func ack() *pb.Response { return &pb.Response{Result: &pb.Response_Ack{Ack: &pb.Ack{}}} }

func ackOr(err error) *pb.Response {
	if err != nil {
		return errResp(err)
	}
	return ack()
}
