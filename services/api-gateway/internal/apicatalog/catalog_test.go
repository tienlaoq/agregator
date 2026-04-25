package apicatalog

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
)

func TestEntry_Write_JSONShape(t *testing.T) {
	rr := httptest.NewRecorder()
	GatewayAuthUnauthorized.Write(rr)
	require.Equal(t, GatewayAuthUnauthorized.HTTP, rr.Code)
	var body map[string]string
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
	assert.Equal(t, GatewayAuthUnauthorized.Code, body["code"])
	assert.Equal(t, GatewayAuthUnauthorized.Message, body["error"])
}

func TestEntry_WithMessage(t *testing.T) {
	base := GatewayUpstreamNotFound
	custom := base.WithMessage("  venue missing  ")
	assert.Equal(t, base.Code, custom.Code)
	assert.Equal(t, base.HTTP, custom.HTTP)
	assert.Equal(t, "venue missing", custom.Message)

	same := base.WithMessage("   ")
	assert.Equal(t, base.Message, same.Message)
}

func TestFromGRPC(t *testing.T) {
	e, ok := FromGRPC(codes.OK)
	assert.False(t, ok)
	assert.Equal(t, "", e.Code)

	nf, ok := FromGRPC(codes.NotFound)
	require.True(t, ok)
	assert.Equal(t, "GATEWAY.UPSTREAM.NOT_FOUND", nf.Code)
}
