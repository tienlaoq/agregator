package grpcutil

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestPgErrToStatus_UniqueViolationDoesNotExposeDetail(t *testing.T) {
	err := &pgconn.PgError{
		Code:    "23505",
		Message: `duplicate key value violates unique constraint "venues_slug_key"`,
		Detail:  "Key (slug)=(baba-yaga) already exists.",
	}

	mapped := pgErrToStatus(err)
	if mapped == nil {
		t.Fatal("expected pg error to be mapped")
	}

	st, ok := status.FromError(mapped)
	if !ok {
		t.Fatal("expected mapped error to be a grpc status")
	}
	if st.Code() != codes.AlreadyExists {
		t.Fatalf("code = %v, want %v", st.Code(), codes.AlreadyExists)
	}
	if st.Message() != "already exists" {
		t.Fatalf("message = %q, want generic already exists", st.Message())
	}
	if strings.Contains(st.Message(), "baba-yaga") || strings.Contains(st.Message(), "venues_slug_key") || strings.Contains(st.Message(), "slug") {
		t.Fatalf("message exposes pg detail: %q", st.Message())
	}
}
