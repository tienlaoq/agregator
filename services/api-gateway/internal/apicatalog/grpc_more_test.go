package apicatalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
)

func TestFromGRPC_AllMappedCodes(t *testing.T) {
	cases := map[codes.Code]Entry{
		codes.InvalidArgument:    GatewayUpstreamInvalidArgument,
		codes.NotFound:           GatewayUpstreamNotFound,
		codes.AlreadyExists:      GatewayUpstreamAlreadyExists,
		codes.Unauthenticated:    GatewayUpstreamUnauthenticated,
		codes.PermissionDenied:   GatewayUpstreamPermissionDenied,
		codes.Unavailable:        GatewayUpstreamUnavailable,
		codes.FailedPrecondition: GatewayUpstreamFailedPrecondition,
		codes.Internal:           GatewayUpstreamInternal,
		codes.Unknown:            GatewayUpstreamUnknown,
	}
	for c, want := range cases {
		got, ok := FromGRPC(c)
		require.Truef(t, ok, "code %v should map", c)
		assert.Equalf(t, want.Code, got.Code, "code %v", c)
	}
}

func TestFromGRPC_UnmappedCodes(t *testing.T) {
	for _, c := range []codes.Code{codes.OK, codes.Canceled, codes.DeadlineExceeded, codes.ResourceExhausted, codes.Aborted} {
		e, ok := FromGRPC(c)
		assert.Falsef(t, ok, "code %v should be unmapped", c)
		assert.Equal(t, "", e.Code)
	}
}

func TestByCode_RoundTrip(t *testing.T) {
	// Every mapped gRPC entry must be resolvable by its own Code string.
	for _, e := range []Entry{
		GatewayAuthUnauthorized,
		GatewayRequestInvalidBody,
		GatewayRequestInvalidJson,
		GatewayUpstreamNotFound,
		GatewayVenueNotFound,
	} {
		got, ok := ByCode(e.Code)
		require.Truef(t, ok, "ByCode(%q) should resolve", e.Code)
		assert.Equal(t, e.Code, got.Code)
		assert.Equal(t, e.HTTP, got.HTTP)
	}
}

func TestByCode_Unknown(t *testing.T) {
	e, ok := ByCode("NOPE.DOES.NOT.EXIST")
	assert.False(t, ok)
	assert.Equal(t, "", e.Code)
}
