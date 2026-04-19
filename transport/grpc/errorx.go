package grpc

import "github.com/slam0504/go-ddd-core/pkg/errorsx"

// StatusCode is a gRPC status code expressed as a uint32 so this package
// stays independent of google.golang.org/grpc/codes. The numeric values
// match google.golang.org/grpc/codes verbatim — adapters can cast directly:
//
//	codes.Code(grpc.CodeFromErrorsx(errorsxCode))
const (
	StatusOK                 uint32 = 0
	StatusCancelled          uint32 = 1
	StatusUnknown            uint32 = 2
	StatusInvalidArgument    uint32 = 3
	StatusDeadlineExceeded   uint32 = 4
	StatusNotFound           uint32 = 5
	StatusAlreadyExists      uint32 = 6
	StatusPermissionDenied   uint32 = 7
	StatusResourceExhausted  uint32 = 8
	StatusFailedPrecondition uint32 = 9
	StatusAborted            uint32 = 10
	StatusOutOfRange         uint32 = 11
	StatusUnimplemented      uint32 = 12
	StatusInternal           uint32 = 13
	StatusUnavailable        uint32 = 14
	StatusDataLoss           uint32 = 15
	StatusUnauthenticated    uint32 = 16
)

// statusByErrorsxCode maps every errorsx.Code to a gRPC status code. The
// mapping is opinionated; adapters needing different conventions should build
// their own mapper rather than mutate this table.
var statusByErrorsxCode = map[errorsx.Code]uint32{
	errorsx.CodeUnknown:         StatusUnknown,
	errorsx.CodeInvalidArgument: StatusInvalidArgument,
	errorsx.CodeNotFound:        StatusNotFound,
	errorsx.CodeAlreadyExists:   StatusAlreadyExists,
	errorsx.CodeUnauthorized:    StatusUnauthenticated,
	errorsx.CodeForbidden:       StatusPermissionDenied,
	errorsx.CodeConflict:        StatusAborted,
	errorsx.CodeRateLimited:     StatusResourceExhausted,
	errorsx.CodeInternal:        StatusInternal,
	errorsx.CodeUnavailable:     StatusUnavailable,
}

// CodeFromErrorsx converts an errorsx.Code into a gRPC status code value.
// Unknown errorsx codes return StatusUnknown so latent codes never silently
// surface as OK.
func CodeFromErrorsx(c errorsx.Code) uint32 {
	if s, ok := statusByErrorsxCode[c]; ok {
		return s
	}
	return StatusUnknown
}
