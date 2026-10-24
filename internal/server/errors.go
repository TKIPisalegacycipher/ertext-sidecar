package server

import (
	"context"
	"errors"
	"net/http"

	"go.mau.fi/mautrix-gmessages/pkg/libgm"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/events"

	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/pb"
	"github.com/TKIPisalegacycipher/ertext-sidecar/internal/translate"
)

var errNotConnected = errors.New("not connected: send Connect or StartPairing first")

// errSendFailed wraps a non-success status from the phone.
var errSendFailed = errors.New("send failed")

// errInternal is what a request gets when its handler panicked.
var errInternal = errors.New("internal error")

// isFatalHTTPError mirrors the HTTP cases of libgm's isFatalRefreshError: Connect's token
// refresh fails with an HTTPError carrying 401/403/404 when Google rejects the pairing.
func isFatalHTTPError(err error) bool {
	var httpErr events.HTTPError
	if !errors.As(err, &httpErr) || httpErr.Resp == nil {
		return false
	}
	switch httpErr.Resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
		return true
	}
	return false
}

func codeFor(err error) pb.ErrorCode {
	switch {
	case errors.Is(err, errNotConnected):
		return pb.ErrorCode_ERROR_CODE_NOT_CONNECTED
	case errors.Is(err, translate.ErrBadRequest):
		return pb.ErrorCode_ERROR_CODE_BAD_REQUEST
	case translate.IsCredentialError(err), isFatalHTTPError(err):
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
