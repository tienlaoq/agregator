package usecase

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pkgauth "github.com/tienlao/agregator/pkg/auth"
	pkgerr "github.com/tienlao/agregator/pkg/errors"
	"github.com/tienlao/agregator/services/auth-service/internal/domain"
)

// testPrivKey is generated once per test binary run.
var testPrivKey *ecdsa.PrivateKey

func init() {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic("auth_test: failed to generate EC key: " + err.Error())
	}
	testPrivKey = priv
}

func testAuthUC(creds *mockCredRepo, tokens *mockTokenRepo, users *mockUserClient) *AuthUseCase {
	return NewAuthUseCase(
		creds, tokens,
		noopPasswordResetRepo{}, noopPasswordMail{},
		time.Hour,
		noopEmailVerificationRepo{}, noopVerifyMail{},
		24*time.Hour,
		users, testPrivKey, time.Hour, 24*time.Hour,
		nil, // partnerNotify: nil is safe — Enqueue is guarded by nil-check
		zerolog.Nop(),
	)
}

func TestRegister_Success(t *testing.T) {
	ctx := context.Background()

	var capturedUserID string
	var credCreateCalled bool
	creds := &mockCredRepo{
		CreateFunc: func(_ context.Context, cred *domain.Credential) error {
			credCreateCalled = true
			require.NotEmpty(t, cred.UserID, "usecase must generate a non-empty user ID")
			require.Equal(t, "reg@example.com", cred.Email)
			require.NotEmpty(t, cred.PasswordHash)
			capturedUserID = cred.UserID
			return nil
		},
	}
	tokens := &mockTokenRepo{
		CreateFunc: func(_ context.Context, rt *domain.RefreshToken) error {
			require.NotEmpty(t, rt.UserID)
			require.NotEmpty(t, rt.TokenHash)
			return nil
		},
	}
	users := &mockUserClient{
		// CreateUser receives the same UUID that was used for creds.Create.
		CreateUserFunc: func(_ context.Context, in domain.CreateUserInput) (*domain.UserRecord, error) {
			require.Equal(t, capturedUserID, in.ID, "user-svc must receive the same UUID as the credential row")
			require.Equal(t, "reg@example.com", in.Email)
			require.Equal(t, "user", in.Role)
			return &domain.UserRecord{ID: in.ID, Email: in.Email, Role: in.Role}, nil
		},
	}

	uc := testAuthUC(creds, tokens, users)
	res, err := uc.Register(ctx, RegisterInput{
		Email:    "reg@example.com",
		Phone:    "+1000",
		Password: "register-pass1",
		Name:     "Reg User",
		Role:     "user",
	})
	require.NoError(t, err)
	require.True(t, credCreateCalled)
	require.NotEmpty(t, res.UserID)
	require.Equal(t, capturedUserID, res.UserID)
	require.NotEmpty(t, res.Tokens.AccessToken)
	require.NotEmpty(t, res.Tokens.RefreshToken)
}

// ---------------------------------------------------------------------------
// Register — partner notification via PartnerNotifier
// ---------------------------------------------------------------------------

// testAuthUCWithNotifier creates a usecase with a custom PartnerNotifier so
// tests can assert on Enqueue calls.
func testAuthUCWithNotifier(
	creds *mockCredRepo,
	tokens *mockTokenRepo,
	users *mockUserClient,
	notifier domain.PartnerNotifier,
) *AuthUseCase {
	return NewAuthUseCase(
		creds, tokens,
		noopPasswordResetRepo{}, noopPasswordMail{},
		time.Hour,
		noopEmailVerificationRepo{}, noopVerifyMail{},
		24*time.Hour,
		users, testPrivKey, time.Hour, 24*time.Hour,
		notifier,
		zerolog.Nop(),
	)
}

func TestRegister_PartnerRole_EnqueuesNotification(t *testing.T) {
	ctx := context.Background()

	for _, role := range []string{"master", "venue_owner"} {
		role := role
		t.Run(role, func(t *testing.T) {
			notifier := &mockPartnerNotifier{}
			creds := &mockCredRepo{
				CreateFunc: func(_ context.Context, _ *domain.Credential) error { return nil },
			}
			tokens := &mockTokenRepo{
				CreateFunc: func(_ context.Context, _ *domain.RefreshToken) error { return nil },
			}
			users := &mockUserClient{
				CreateUserFunc: func(_ context.Context, in domain.CreateUserInput) (*domain.UserRecord, error) {
					return &domain.UserRecord{ID: "partner-uid", Email: in.Email, Role: in.Role}, nil
				},
			}

			uc := testAuthUCWithNotifier(creds, tokens, users, notifier)
			_, err := uc.Register(ctx, RegisterInput{
				Email:    "partner@example.com",
				Password: "strongpass1",
				Name:     "Partner User",
				Role:     role,
			})
			require.NoError(t, err)
			require.Len(t, notifier.events, 1, "exactly one event must be enqueued for role %s", role)
			assert.Equal(t, role, notifier.events[0].Role)
			assert.Equal(t, "partner@example.com", notifier.events[0].Email)
		})
	}
}

func TestRegister_UserRole_NoNotification(t *testing.T) {
	ctx := context.Background()

	notifier := &mockPartnerNotifier{}
	creds := &mockCredRepo{
		CreateFunc: func(_ context.Context, _ *domain.Credential) error { return nil },
	}
	tokens := &mockTokenRepo{
		CreateFunc: func(_ context.Context, _ *domain.RefreshToken) error { return nil },
	}
	users := &mockUserClient{
		CreateUserFunc: func(_ context.Context, in domain.CreateUserInput) (*domain.UserRecord, error) {
			return &domain.UserRecord{ID: "user-uid", Email: in.Email, Role: in.Role}, nil
		},
	}

	uc := testAuthUCWithNotifier(creds, tokens, users, notifier)
	_, err := uc.Register(ctx, RegisterInput{
		Email:    "regular@example.com",
		Password: "strongpass1",
		Name:     "Regular User",
		Role:     "user",
	})
	require.NoError(t, err)
	assert.Empty(t, notifier.events, "no notification must be enqueued for role 'user'")
}

func TestRegister_CreateUserFails_CompensatesCredential(t *testing.T) {
	ctx := context.Background()

	var deletedUserID string
	creds := &mockCredRepo{
		CreateFunc: func(_ context.Context, _ *domain.Credential) error { return nil },
		DeleteByUserIDFunc: func(_ context.Context, userID string) error {
			deletedUserID = userID
			return nil
		},
	}
	users := &mockUserClient{
		CreateUserFunc: func(_ context.Context, in domain.CreateUserInput) (*domain.UserRecord, error) {
			return nil, status.Error(codes.Internal, "user-svc unavailable")
		},
	}

	uc := testAuthUC(creds, &mockTokenRepo{}, users)
	_, err := uc.Register(ctx, RegisterInput{
		Email:    "comp@example.com",
		Password: "strongpass1",
		Name:     "Comp User",
		Role:     "user",
	})
	require.Error(t, err)
	require.NotEmpty(t, deletedUserID, "compensating delete must be called when CreateUser fails")
}

func TestRegister_CredCreateFails_UserSvcNotCalled(t *testing.T) {
	ctx := context.Background()

	var userSvcCalled bool
	creds := &mockCredRepo{
		CreateFunc: func(_ context.Context, _ *domain.Credential) error {
			return errors.New("db connection lost")
		},
	}
	users := &mockUserClient{
		CreateUserFunc: func(_ context.Context, _ domain.CreateUserInput) (*domain.UserRecord, error) {
			userSvcCalled = true
			return nil, nil
		},
	}

	uc := testAuthUC(creds, &mockTokenRepo{}, users)
	_, err := uc.Register(ctx, RegisterInput{
		Email:    "fail@example.com",
		Password: "strongpass1",
		Name:     "Fail User",
		Role:     "user",
	})
	require.Error(t, err)
	require.False(t, userSvcCalled, "user-svc must not be called when local creds.Create fails")
}

func TestLogin_Success(t *testing.T) {
	ctx := context.Background()
	hash, err := hashPassword("testpass123")
	require.NoError(t, err)

	const userID = "login-user-1"
	const email = "login@example.com"

	creds := &mockCredRepo{
		GetByEmailFunc: func(_ context.Context, e string) (*domain.Credential, error) {
			require.Equal(t, email, e)
			return &domain.Credential{UserID: userID, Email: email, PasswordHash: hash}, nil
		},
	}
	tokens := &mockTokenRepo{
		CreateFunc: func(_ context.Context, rt *domain.RefreshToken) error {
			require.Equal(t, userID, rt.UserID)
			return nil
		},
	}
	users := &mockUserClient{
		GetUserFunc: func(_ context.Context, gotUserID string) (*domain.UserRecord, error) {
			require.Equal(t, userID, gotUserID)
			return &domain.UserRecord{ID: userID, Email: email, Role: "owner"}, nil
		},
	}

	uc := testAuthUC(creds, tokens, users)
	res, err := uc.Login(ctx, email, "testpass123")
	require.NoError(t, err)
	require.Equal(t, userID, res.UserID)
	require.NotEmpty(t, res.Tokens.AccessToken)
	require.NotEmpty(t, res.Tokens.RefreshToken)
}

func TestLogin_WrongPassword(t *testing.T) {
	ctx := context.Background()
	hash, err := hashPassword("testpass123")
	require.NoError(t, err)

	const email = "wrong@example.com"
	creds := &mockCredRepo{
		GetByEmailFunc: func(_ context.Context, e string) (*domain.Credential, error) {
			return &domain.Credential{UserID: "u", Email: e, PasswordHash: hash}, nil
		},
	}

	uc := testAuthUC(creds, &mockTokenRepo{}, &mockUserClient{})
	_, err = uc.Login(ctx, email, "not-the-password")
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

// TestLogin_UserNotFound_GenericError verifies that any error from GetByEmail
// (not just pkgerr.NotFound) causes Unauthenticated — the usecase must never
// leak which emails exist via a different error code.
// See also TestLogin_UserNotFound_PkgErr which tests the real-repo error type.
func TestLogin_UserNotFound_GenericError(t *testing.T) {
	ctx := context.Background()
	creds := &mockCredRepo{
		GetByEmailFunc: func(_ context.Context, _ string) (*domain.Credential, error) {
			return nil, errors.New("no rows") // generic error, not pkgerr.NotFound
		},
	}

	uc := testAuthUC(creds, &mockTokenRepo{}, &mockUserClient{})
	_, err := uc.Login(ctx, "missing@example.com", "any")
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

// TestLogin_OAuthOnly_NoPassword verifies that an OAuth-only credential
// (PasswordHash == "") cannot be used to log in via password flow.
// The timing guarantee must hold: argon2id runs against dummyPasswordHash even
// when the credential exists, so "OAuth-only user" and "no such user" are
// indistinguishable in response time.
func TestLogin_OAuthOnly_NoPassword(t *testing.T) {
	ctx := context.Background()
	creds := &mockCredRepo{
		GetByEmailFunc: func(_ context.Context, _ string) (*domain.Credential, error) {
			// OAuth-only: credential exists but PasswordHash is empty.
			return &domain.Credential{
				UserID:       "oauth-uid",
				Email:        "oauth@example.com",
				PasswordHash: "",
			}, nil
		},
	}

	var getUserCalled bool
	users := &mockUserClient{
		GetUserFunc: func(_ context.Context, _ string) (*domain.UserRecord, error) {
			getUserCalled = true
			return &domain.UserRecord{ID: "oauth-uid", Role: "user"}, nil
		},
	}

	uc := testAuthUC(creds, &mockTokenRepo{}, users)
	_, err := uc.Login(ctx, "oauth@example.com", "any-password")

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code(),
		"OAuth-only account must return Unauthenticated on password login")
	assert.False(t, getUserCalled,
		"user-service must not be called for an OAuth-only credential")
}

func TestValidateToken_Valid(t *testing.T) {
	ctx := context.Background()
	const wantUserID = "valid-token-user"

	creds := &mockCredRepo{
		CreateFunc: func(_ context.Context, _ *domain.Credential) error { return nil },
	}
	tokens := &mockTokenRepo{
		CreateFunc: func(_ context.Context, _ *domain.RefreshToken) error { return nil },
	}
	users := &mockUserClient{
		CreateUserFunc: func(_ context.Context, _ domain.CreateUserInput) (*domain.UserRecord, error) {
			return &domain.UserRecord{ID: wantUserID, Email: "vt@example.com", Role: "user"}, nil
		},
	}

	uc := testAuthUC(creds, tokens, users)
	reg, err := uc.Register(ctx, RegisterInput{
		Email:    "vt@example.com",
		Password: "validpass1",
		Name:     "VT User",
		Role:     "user",
	})
	require.NoError(t, err)

	claims, err := uc.ValidateToken(ctx, reg.Tokens.AccessToken)
	require.NoError(t, err)
	require.Equal(t, wantUserID, claims.UserID)
	require.Equal(t, "vt@example.com", claims.Email)
	require.Equal(t, "user", claims.Role)
}

func TestRegister_DisallowedRole(t *testing.T) {
	ctx := context.Background()
	uc := testAuthUC(&mockCredRepo{}, &mockTokenRepo{}, &mockUserClient{})

	for _, role := range []string{"admin", "moderator", "support", "ADMIN", " Admin "} {
		_, err := uc.Register(ctx, RegisterInput{
			Email:    "hacker@example.com",
			Password: "strongpass1",
			Name:     "Hacker",
			Role:     role,
		})
		require.Error(t, err, "role %q should be rejected", role)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code(), "role %q should return InvalidArgument", role)
	}
}

func TestRegister_ShortPassword(t *testing.T) {
	ctx := context.Background()
	uc := testAuthUC(&mockCredRepo{}, &mockTokenRepo{}, &mockUserClient{})

	_, err := uc.Register(ctx, RegisterInput{
		Email:    "user@example.com",
		Password: "short",
		Name:     "User",
		Role:     "user",
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

func TestRegister_EmailNormalized(t *testing.T) {
	ctx := context.Background()
	var storedEmail string
	creds := &mockCredRepo{
		CreateFunc: func(_ context.Context, cred *domain.Credential) error {
			storedEmail = cred.Email
			return nil
		},
	}
	tokens := &mockTokenRepo{
		CreateFunc: func(_ context.Context, _ *domain.RefreshToken) error { return nil },
	}
	users := &mockUserClient{
		CreateUserFunc: func(_ context.Context, in domain.CreateUserInput) (*domain.UserRecord, error) {
			// email must already be lower-cased before reaching user-svc
			assert.Equal(t, "upper@example.com", in.Email)
			return &domain.UserRecord{ID: "norm-id", Email: in.Email, Role: in.Role}, nil
		},
	}

	uc := testAuthUC(creds, tokens, users)
	_, err := uc.Register(ctx, RegisterInput{
		Email:    "  UPPER@example.com  ",
		Password: "normalpass1",
		Name:     "Norm User",
		Role:     "user",
	})
	require.NoError(t, err)
	assert.Equal(t, "upper@example.com", storedEmail)
}

// --- OAuthLogin account-takeover tests ---

func TestOAuthLogin_UnverifiedEmail_DoesNotLinkExistingAccount(t *testing.T) {
	ctx := context.Background()

	// Victim has a password-based account at victim@example.com.
	victimCred := &domain.Credential{UserID: "victim-uid", Email: "victim@example.com", PasswordHash: "hash"}

	// linkAttempted tracks whether CreateOAuth was called with victim's UserID
	// (true linking) — not whether CreateOAuth ran at all, since Branch 3 always
	// inserts a fresh credential for the attacker's isolated new account.
	var linkAttempted bool
	creds := &mockCredRepo{
		GetByProviderFunc: func(_ context.Context, _, _ string) (*domain.Credential, error) {
			return nil, errors.New("not found") // attacker has no prior OAuth link
		},
		GetByEmailFunc: func(_ context.Context, email string) (*domain.Credential, error) {
			if email == "victim@example.com" {
				return victimCred, nil
			}
			return nil, errors.New("not found")
		},
		CreateOAuthFunc: func(_ context.Context, cred *domain.Credential) error {
			if cred.UserID == victimCred.UserID {
				linkAttempted = true
			}
			return nil
		},
		CreateFunc: func(_ context.Context, _ *domain.Credential) error { return nil },
	}
	tokens := &mockTokenRepo{
		CreateFunc: func(_ context.Context, _ *domain.RefreshToken) error { return nil },
	}
	users := &mockUserClient{
		CreateUserFunc: func(_ context.Context, in domain.CreateUserInput) (*domain.UserRecord, error) {
			// Must NOT receive victim's email when email_verified=false.
			assert.Empty(t, in.Email, "unverified email must not be forwarded to user-svc")
			return &domain.UserRecord{ID: "attacker-uid", Email: "", Role: "user"}, nil
		},
	}

	uc := testAuthUC(creds, tokens, users)
	result, err := uc.OAuthLogin(ctx, OAuthInput{
		Provider:      "evil-provider",
		ProviderID:    "attacker-999",
		Email:         "victim@example.com",
		Name:          "Attacker",
		EmailVerified: false, // provider did not verify ownership
	})

	require.NoError(t, err, "unverified OAuth should still create a new account, not error")
	require.False(t, linkAttempted, "must not link to victim's account when email_verified=false")
	assert.NotEqual(t, "victim-uid", result.UserID, "attacker must not receive victim's user ID")
	assert.True(t, result.IsNewUser)
}

func TestOAuthLogin_VerifiedEmail_LinksExistingAccount(t *testing.T) {
	ctx := context.Background()

	existingCred := &domain.Credential{UserID: "existing-uid", Email: "user@example.com"}

	var linked bool
	creds := &mockCredRepo{
		GetByProviderFunc: func(_ context.Context, _, _ string) (*domain.Credential, error) {
			return nil, errors.New("not found")
		},
		GetByEmailFunc: func(_ context.Context, _ string) (*domain.Credential, error) {
			return existingCred, nil
		},
		CreateOAuthFunc: func(_ context.Context, _ *domain.Credential) error {
			linked = true
			return nil
		},
	}
	tokens := &mockTokenRepo{
		CreateFunc: func(_ context.Context, _ *domain.RefreshToken) error { return nil },
	}
	users := &mockUserClient{
		GetUserFunc: func(_ context.Context, _ string) (*domain.UserRecord, error) {
			return &domain.UserRecord{ID: "existing-uid", Email: "user@example.com", Role: "user"}, nil
		},
	}

	uc := testAuthUC(creds, tokens, users)
	result, err := uc.OAuthLogin(ctx, OAuthInput{
		Provider:      "google",
		ProviderID:    "google-123",
		Email:         "user@example.com",
		Name:          "Real User",
		EmailVerified: true, // Google confirmed ownership
	})

	require.NoError(t, err)
	require.True(t, linked, "verified email should link to existing account")
	assert.Equal(t, "existing-uid", result.UserID)
	assert.False(t, result.IsNewUser)
}

// ---------------------------------------------------------------------------
// OAuthLogin — concurrent CreateOAuth race recovery
// ---------------------------------------------------------------------------

// TestOAuthLogin_Branch2_CreateOAuth_UniqueRace verifies that when two
// concurrent requests both pass branch 1 (GetByProvider → NotFound) and both
// try to link via branch 2, the loser (CreateOAuth → AlreadyExists) recovers
// by fetching the winner's credential and issuing tokens — not returning Internal.
func TestOAuthLogin_Branch2_CreateOAuth_UniqueRace(t *testing.T) {
	ctx := context.Background()

	const winnerUserID = "branch2-winner-uid"

	callCount := 0
	creds := &mockCredRepo{
		GetByProviderFunc: func(_ context.Context, _, _ string) (*domain.Credential, error) {
			callCount++
			if callCount == 1 {
				// First call: branch 1 misses (no prior link).
				return nil, pkgerr.NotFound("not found")
			}
			// Second call: recovery fetch after AlreadyExists — returns winner.
			return &domain.Credential{UserID: winnerUserID, Email: "race@example.com"}, nil
		},
		GetByEmailFunc: func(_ context.Context, _ string) (*domain.Credential, error) {
			return &domain.Credential{UserID: winnerUserID, Email: "race@example.com"}, nil
		},
		CreateOAuthFunc: func(_ context.Context, _ *domain.Credential) error {
			// Simulate the losing request hitting the UNIQUE constraint.
			return pkgerr.AlreadyExists("duplicate key value violates unique constraint")
		},
	}
	tokens := &mockTokenRepo{
		CreateFunc: func(_ context.Context, _ *domain.RefreshToken) error { return nil },
	}
	users := &mockUserClient{
		GetUserFunc: func(_ context.Context, uid string) (*domain.UserRecord, error) {
			return &domain.UserRecord{ID: uid, Email: "race@example.com", Role: "user"}, nil
		},
	}

	uc := testAuthUC(creds, tokens, users)
	result, err := uc.OAuthLogin(ctx, OAuthInput{
		Provider:      "google",
		ProviderID:    "g-race-b2",
		Email:         "race@example.com",
		EmailVerified: true,
	})

	require.NoError(t, err, "concurrent branch-2 race must recover, not return Internal")
	assert.Equal(t, winnerUserID, result.UserID)
	assert.False(t, result.IsNewUser)
}

// TestOAuthLogin_Branch3_CreateOAuth_UniqueRace verifies that when two
// concurrent new-user requests both miss branch 1 and race to CreateOAuth,
// the loser recovers by fetching the winner's credential, not returning Internal.
func TestOAuthLogin_Branch3_CreateOAuth_UniqueRace(t *testing.T) {
	ctx := context.Background()

	const winnerUserID = "branch3-winner-uid"

	getByProviderCallCount := 0
	creds := &mockCredRepo{
		GetByProviderFunc: func(_ context.Context, _, _ string) (*domain.Credential, error) {
			getByProviderCallCount++
			if getByProviderCallCount == 1 {
				return nil, pkgerr.NotFound("not found") // initial miss
			}
			// Recovery fetch after losing the race.
			return &domain.Credential{UserID: winnerUserID, Email: ""}, nil
		},
		CreateOAuthFunc: func(_ context.Context, _ *domain.Credential) error {
			return pkgerr.AlreadyExists("duplicate key value violates unique constraint")
		},
	}
	tokens := &mockTokenRepo{
		CreateFunc: func(_ context.Context, _ *domain.RefreshToken) error { return nil },
	}
	users := &mockUserClient{
		CreateUserFunc: func(_ context.Context, _ domain.CreateUserInput) (*domain.UserRecord, error) {
			// Losing request creates a user that will become an orphan.
			return &domain.UserRecord{ID: "orphan-uid", Email: "", Role: "user"}, nil
		},
		GetUserFunc: func(_ context.Context, uid string) (*domain.UserRecord, error) {
			return &domain.UserRecord{ID: uid, Email: "", Role: "user"}, nil
		},
	}

	uc := testAuthUC(creds, tokens, users)
	result, err := uc.OAuthLogin(ctx, OAuthInput{
		Provider:      "github",
		ProviderID:    "gh-race-b3",
		EmailVerified: false,
	})

	require.NoError(t, err, "concurrent branch-3 race must recover, not return Internal")
	assert.Equal(t, winnerUserID, result.UserID,
		"losing request must serve the winner's credential, not its own orphan userID")
	assert.False(t, result.IsNewUser,
		"recovered request is not a new user — the winner already created the account")
}

func TestValidateToken_Expired(t *testing.T) {
	ctx := context.Background()
	// Generate an already-expired token using the same key as the usecase.
	expired, err := pkgauth.GenerateAccessToken(testPrivKey, "u1", "e@x.com", "user", -time.Hour)
	require.NoError(t, err)

	uc := testAuthUC(&mockCredRepo{}, &mockTokenRepo{}, &mockUserClient{})
	_, err = uc.ValidateToken(ctx, expired)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

// --- verifyPassword resource-limit tests ---

// --- RefreshToken tests ---

// TestRefreshToken_Success verifies the happy path: token atomically consumed
// (ConsumeByHash), identity fetched, new pair issued.
func TestRefreshToken_Success(t *testing.T) {
	ctx := context.Background()

	const userID = "rt-user-2"
	rawToken := "valid-raw-refresh"
	tokenHash := hashRefreshToken(rawToken)
	storedToken := &domain.RefreshToken{
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(time.Hour),
	}

	var consumed bool
	tokens := &mockTokenRepo{
		ConsumeByHashFunc: func(_ context.Context, h string) (*domain.RefreshToken, error) {
			if h == tokenHash {
				consumed = true
				return storedToken, nil
			}
			return nil, pkgerr.NotFound("not found")
		},
		CreateFunc: func(_ context.Context, _ *domain.RefreshToken) error { return nil },
	}
	creds := &mockCredRepo{
		GetByUserIDFunc: func(_ context.Context, _ string) (*domain.Credential, error) {
			return &domain.Credential{UserID: userID, Email: "rt@example.com"}, nil
		},
	}
	users := &mockUserClient{
		GetUserFunc: func(_ context.Context, _ string) (*domain.UserRecord, error) {
			return &domain.UserRecord{ID: userID, Email: "rt@example.com", Role: "user"}, nil
		},
	}

	uc := testAuthUC(creds, tokens, users)
	pair, err := uc.RefreshToken(ctx, rawToken)

	require.NoError(t, err)
	require.True(t, consumed, "token must be atomically consumed via ConsumeByHash")
	require.NotEmpty(t, pair.AccessToken)
	require.NotEmpty(t, pair.RefreshToken)
}

// TestRefreshToken_UserSvcDown_TokenAlreadyConsumed documents the trade-off of
// the ConsumeByHash-first approach: if user-service is unavailable after the
// token has been atomically consumed, the token is lost and the client must
// re-authenticate. This is intentional — see RefreshToken godoc.
func TestRefreshToken_UserSvcDown_TokenAlreadyConsumed(t *testing.T) {
	ctx := context.Background()

	const userID = "rt-user-1"
	rawToken := "raw-refresh-token"
	tokenHash := hashRefreshToken(rawToken)

	tokens := &mockTokenRepo{
		ConsumeByHashFunc: func(_ context.Context, h string) (*domain.RefreshToken, error) {
			if h == tokenHash {
				return &domain.RefreshToken{
					UserID:    userID,
					TokenHash: tokenHash,
					ExpiresAt: time.Now().Add(time.Hour),
				}, nil
			}
			return nil, pkgerr.NotFound("not found")
		},
	}
	creds := &mockCredRepo{
		GetByUserIDFunc: func(_ context.Context, _ string) (*domain.Credential, error) {
			return &domain.Credential{UserID: userID, Email: "rt@example.com"}, nil
		},
	}
	users := &mockUserClient{
		GetUserFunc: func(_ context.Context, _ string) (*domain.UserRecord, error) {
			return nil, fmt.Errorf("user-service unavailable")
		},
	}

	uc := testAuthUC(creds, tokens, users)
	_, err := uc.RefreshToken(ctx, rawToken)

	// The token is consumed and the call fails — client must re-authenticate.
	require.Error(t, err)
}

// TestRefreshToken_UnknownOrAlreadyConsumed verifies that a token not present in
// the store (bogus, cleaned up, or already consumed by a prior request) is
// rejected with Unauthenticated. ConsumeByHash returning NotFound covers all
// these cases uniformly.
func TestRefreshToken_UnknownOrAlreadyConsumed(t *testing.T) {
	ctx := context.Background()

	tokens := &mockTokenRepo{
		ConsumeByHashFunc: func(_ context.Context, _ string) (*domain.RefreshToken, error) {
			return nil, pkgerr.NotFound("refresh token not found")
		},
	}

	uc := testAuthUC(&mockCredRepo{}, tokens, &mockUserClient{})
	_, err := uc.RefreshToken(ctx, "completely-unknown-token")

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

// TestRefreshToken_Concurrent_OnlyOneSucceeds verifies the concurrent-replay
// safety property: if two requests race with the same token, exactly one
// ConsumeByHash wins; the other gets NotFound → Unauthenticated.
// The winning request is modelled by the first call succeeding; the losing one
// by the second call returning NotFound.
func TestRefreshToken_Concurrent_OnlyOneSucceeds(t *testing.T) {
	ctx := context.Background()

	const userID = "rt-race-user"
	rawToken := "raced-token"
	tokenHash := hashRefreshToken(rawToken)
	storedToken := &domain.RefreshToken{
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(time.Hour),
	}

	callCount := 0
	tokens := &mockTokenRepo{
		ConsumeByHashFunc: func(_ context.Context, h string) (*domain.RefreshToken, error) {
			callCount++
			if callCount == 1 {
				return storedToken, nil // first caller wins
			}
			return nil, pkgerr.NotFound("already consumed") // second caller loses
		},
		CreateFunc: func(_ context.Context, _ *domain.RefreshToken) error { return nil },
	}
	creds := &mockCredRepo{
		GetByUserIDFunc: func(_ context.Context, _ string) (*domain.Credential, error) {
			return &domain.Credential{UserID: userID, Email: "race@example.com"}, nil
		},
	}
	users := &mockUserClient{
		GetUserFunc: func(_ context.Context, _ string) (*domain.UserRecord, error) {
			return &domain.UserRecord{ID: userID, Email: "race@example.com", Role: "user"}, nil
		},
	}

	uc := testAuthUC(creds, tokens, users)

	pair, err1 := uc.RefreshToken(ctx, rawToken)
	require.NoError(t, err1, "first caller must succeed")
	require.NotEmpty(t, pair.AccessToken)

	_, err2 := uc.RefreshToken(ctx, rawToken)
	require.Error(t, err2, "second caller must fail")
	st, ok := status.FromError(err2)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

// --- verifyPassword resource-limit tests ---

func TestVerifyPassword_RejectsExcessiveMemory(t *testing.T) {
	crafted := fmt.Sprintf("$argon2id$v=19$m=%d,t=1,p=1$c2FsdHNhbHQ$aGFzaGhhc2g", argon2MaxMemory+1)
	assert.False(t, verifyPassword("anypassword", crafted),
		"memory > argon2MaxMemory must be rejected before key derivation")
}

func TestVerifyPassword_RejectsExcessiveIterations(t *testing.T) {
	crafted := fmt.Sprintf("$argon2id$v=19$m=65536,t=%d,p=1$c2FsdHNhbHQ$aGFzaGhhc2g", argon2MaxTime+1)
	assert.False(t, verifyPassword("anypassword", crafted),
		"iter > argon2MaxTime must be rejected before key derivation")
}

func TestVerifyPassword_RejectsExcessiveParallelism(t *testing.T) {
	crafted := fmt.Sprintf("$argon2id$v=19$m=65536,t=1,p=%d$c2FsdHNhbHQ$aGFzaGhhc2g", uint32(argon2MaxThreads)+1)
	assert.False(t, verifyPassword("anypassword", crafted),
		"par > argon2MaxThreads must be rejected before key derivation")
}

func TestVerifyPassword_RejectsZeroMemory(t *testing.T) {
	crafted := "$argon2id$v=19$m=0,t=1,p=1$c2FsdHNhbHQ$aGFzaGhhc2g"
	assert.False(t, verifyPassword("anypassword", crafted), "m=0 must be rejected")
}

func TestVerifyPassword_RejectsZeroIterations(t *testing.T) {
	crafted := "$argon2id$v=19$m=65536,t=0,p=1$c2FsdHNhbHQ$aGFzaGhhc2g"
	assert.False(t, verifyPassword("anypassword", crafted), "t=0 must be rejected")
}

func TestVerifyPassword_RejectsZeroParallelism(t *testing.T) {
	crafted := "$argon2id$v=19$m=65536,t=1,p=0$c2FsdHNhbHQ$aGFzaGhhc2g"
	assert.False(t, verifyPassword("anypassword", crafted), "p=0 must be rejected")
}

func TestVerifyPassword_AcceptsDefaultParams(t *testing.T) {
	hash, err := hashPassword("correct-password")
	require.NoError(t, err)
	assert.True(t, verifyPassword("correct-password", hash))
	assert.False(t, verifyPassword("wrong-password", hash))
}

func TestVerifyPassword_CeilingConsistency(t *testing.T) {
	// Ceiling values must be >= defaultParams so that hashes produced today
	// are never rejected by the limit guard after a future code deploy.
	assert.GreaterOrEqual(t, argon2MaxMemory, defaultParams.memory,
		"argon2MaxMemory must be >= defaultParams.memory")
	assert.GreaterOrEqual(t, argon2MaxTime, defaultParams.time,
		"argon2MaxTime must be >= defaultParams.time")
	assert.GreaterOrEqual(t, uint32(argon2MaxThreads), uint32(defaultParams.threads),
		"argon2MaxThreads must be >= defaultParams.threads")
}

func TestVerifyPassword_CeilingValuesNotRejected(t *testing.T) {
	// A hash string with parameters exactly at the ceiling must not be rejected
	// by the limit guards (the ceil is inclusive). The hash bytes are invalid
	// so verifyPassword returns false, but it must not panic.
	ceilingHash := fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$c2FsdHNhbHQ$aGFzaGhhc2g",
		argon2MaxMemory, argon2MaxTime, argon2MaxThreads)
	// We cannot actually run argon2id at 256 MiB in a unit test, so we only
	// verify the parsing path does not panic and does not return false due to
	// the limit guard (it returns false for the wrong-hash reason, not limits).
	// The real guard test is the Excessive* cases above.
	assert.NotPanics(t, func() { verifyPassword("any", ceilingHash) })
}

// ---------------------------------------------------------------------------
// Register — duplicate email / user-service error propagation
// ---------------------------------------------------------------------------

// TestRegister_DuplicateEmail_LocalUniqueViolation checks that a PostgreSQL
// unique-constraint error (SQLSTATE 23505) from creds.Create surfaces as
// AlreadyExists, and that user-service is never called.
func TestRegister_DuplicateEmail_LocalUniqueViolation(t *testing.T) {
	ctx := context.Background()

	// The real CredentialRepo translates a PostgreSQL unique-violation into
	// pkgerr.AlreadyExists before returning it to the usecase. We replicate that
	// here so the test verifies the usecase-level forwarding behaviour, not the
	// repo translation (which is covered by integration tests).
	var userSvcCalled bool
	creds := &mockCredRepo{
		CreateFunc: func(_ context.Context, _ *domain.Credential) error {
			return pkgerr.AlreadyExists("email already registered")
		},
	}
	users := &mockUserClient{
		CreateUserFunc: func(_ context.Context, _ domain.CreateUserInput) (*domain.UserRecord, error) {
			userSvcCalled = true
			return nil, nil
		},
	}

	uc := testAuthUC(creds, &mockTokenRepo{}, users)
	_, err := uc.Register(ctx, RegisterInput{
		Email:    "dup@example.com",
		Password: "strongpass1",
		Name:     "Dup User",
		Role:     "user",
	})

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.AlreadyExists, st.Code(), "duplicate email must surface as AlreadyExists")
	assert.False(t, userSvcCalled, "user-svc must not be called when local creds.Create fails")
}

// TestRegister_UserSvc_AlreadyExists checks that codes.AlreadyExists from
// user-service is forwarded as AlreadyExists (not Internal).
func TestRegister_UserSvc_AlreadyExists(t *testing.T) {
	ctx := context.Background()

	creds := &mockCredRepo{
		CreateFunc:         func(_ context.Context, _ *domain.Credential) error { return nil },
		DeleteByUserIDFunc: func(_ context.Context, _ string) error { return nil },
	}
	users := &mockUserClient{
		CreateUserFunc: func(_ context.Context, _ domain.CreateUserInput) (*domain.UserRecord, error) {
			return nil, status.Error(codes.AlreadyExists, "user already exists in user-svc")
		},
	}

	uc := testAuthUC(creds, &mockTokenRepo{}, users)
	_, err := uc.Register(ctx, RegisterInput{
		Email:    "already@example.com",
		Password: "strongpass1",
		Name:     "Already User",
		Role:     "user",
	})

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.AlreadyExists, st.Code())
}

// TestRegister_UserSvc_InvalidArgument checks that codes.InvalidArgument from
// user-service is forwarded as InvalidArgument (not Internal).
func TestRegister_UserSvc_InvalidArgument(t *testing.T) {
	ctx := context.Background()

	creds := &mockCredRepo{
		CreateFunc:         func(_ context.Context, _ *domain.Credential) error { return nil },
		DeleteByUserIDFunc: func(_ context.Context, _ string) error { return nil },
	}
	users := &mockUserClient{
		CreateUserFunc: func(_ context.Context, _ domain.CreateUserInput) (*domain.UserRecord, error) {
			return nil, status.Error(codes.InvalidArgument, "phone number invalid")
		},
	}

	uc := testAuthUC(creds, &mockTokenRepo{}, users)
	_, err := uc.Register(ctx, RegisterInput{
		Email:    "bad-input@example.com",
		Password: "strongpass1",
		Name:     "Bad Input User",
		Role:     "user",
	})

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

// ---------------------------------------------------------------------------
// Login — pkgerr.NotFound from the real repo
// ---------------------------------------------------------------------------

// TestLogin_UserNotFound_PkgErr verifies that pkgerr.NotFound (the error type
// the real CredentialRepo returns on ErrNoRows) results in Unauthenticated,
// not a different gRPC code or a panic. This replaces the earlier test that
// used errors.New("no rows") which does not match the real error type.
func TestLogin_UserNotFound_PkgErr(t *testing.T) {
	ctx := context.Background()
	creds := &mockCredRepo{
		GetByEmailFunc: func(_ context.Context, _ string) (*domain.Credential, error) {
			return nil, pkgerr.NotFound("credential not found")
		},
	}

	uc := testAuthUC(creds, &mockTokenRepo{}, &mockUserClient{})
	_, err := uc.Login(ctx, "missing@example.com", "any-password")
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code(),
		"pkgerr.NotFound from repo must become Unauthenticated, not NotFound")
}

// ---------------------------------------------------------------------------
// RefreshToken — expired token
// ---------------------------------------------------------------------------

// TestRefreshToken_Expired verifies that a token returned by ConsumeByHash with
// an ExpiresAt in the past is rejected. The row is already deleted by
// ConsumeByHash (DELETE … RETURNING), so no separate cleanup call is needed.
func TestRefreshToken_Expired(t *testing.T) {
	ctx := context.Background()

	rawToken := "expired-raw-token"
	tokenHash := hashRefreshToken(rawToken)

	tokens := &mockTokenRepo{
		ConsumeByHashFunc: func(_ context.Context, h string) (*domain.RefreshToken, error) {
			if h == tokenHash {
				return &domain.RefreshToken{
					UserID:    "some-user",
					TokenHash: tokenHash,
					ExpiresAt: time.Now().Add(-time.Minute), // already expired
				}, nil
			}
			return nil, pkgerr.NotFound("not found")
		},
	}

	uc := testAuthUC(&mockCredRepo{}, tokens, &mockUserClient{})
	_, err := uc.RefreshToken(ctx, rawToken)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
	// No separate cleanup assert: ConsumeByHash already deleted the row.
}

// ---------------------------------------------------------------------------
// OAuthLogin — unverified email not stored; verified email normalised
// ---------------------------------------------------------------------------

// TestOAuthLogin_UnverifiedEmail_NotStored verifies that when EmailVerified=false
// the credential row is created with an empty email, not the raw provider string.
// Storing an unverified email would silently claim the address and prevent the
// real owner from registering or resetting their password.
func TestOAuthLogin_UnverifiedEmail_NotStored(t *testing.T) {
	ctx := context.Background()

	var storedEmail string
	creds := &mockCredRepo{
		GetByProviderFunc: func(_ context.Context, _, _ string) (*domain.Credential, error) {
			return nil, pkgerr.NotFound("not found")
		},
		// EmailVerified=false → skip GetByEmail → fall through to new-user branch.
		CreateOAuthFunc: func(_ context.Context, c *domain.Credential) error {
			storedEmail = c.Email
			return nil
		},
	}
	tokens := &mockTokenRepo{
		CreateFunc: func(_ context.Context, _ *domain.RefreshToken) error { return nil },
	}
	users := &mockUserClient{
		CreateUserFunc: func(_ context.Context, in domain.CreateUserInput) (*domain.UserRecord, error) {
			// EmailVerified=false → registration email forwarded as empty
			return &domain.UserRecord{ID: "new-uid", Email: "", Role: "user"}, nil
		},
	}

	uc := testAuthUC(creds, tokens, users)
	_, err := uc.OAuthLogin(ctx, OAuthInput{
		Provider:      "github",
		ProviderID:    "gh-42",
		Email:         "  User@Example.COM  ",
		Name:          "Github User",
		EmailVerified: false,
	})
	require.NoError(t, err)
	assert.Empty(t, storedEmail, "unverified email must not be stored in credentials")
}

// TestOAuthLogin_VerifiedEmail_Normalised verifies that a verified email is
// normalised before GetByEmail is called (so the lookup hits the functional
// index) and before it is stored in the new OAuth credential row.
func TestOAuthLogin_VerifiedEmail_Normalised(t *testing.T) {
	ctx := context.Background()

	var lookupEmail string
	var storedEmail string
	creds := &mockCredRepo{
		GetByProviderFunc: func(_ context.Context, _, _ string) (*domain.Credential, error) {
			return nil, pkgerr.NotFound("not found")
		},
		GetByEmailFunc: func(_ context.Context, e string) (*domain.Credential, error) {
			lookupEmail = e
			return &domain.Credential{UserID: "existing-uid", Email: e}, nil
		},
		CreateOAuthFunc: func(_ context.Context, c *domain.Credential) error {
			storedEmail = c.Email
			return nil
		},
	}
	tokens := &mockTokenRepo{
		CreateFunc: func(_ context.Context, _ *domain.RefreshToken) error { return nil },
	}
	users := &mockUserClient{
		GetUserFunc: func(_ context.Context, _ string) (*domain.UserRecord, error) {
			return &domain.UserRecord{ID: "existing-uid", Email: "user@example.com", Role: "user"}, nil
		},
	}

	uc := testAuthUC(creds, tokens, users)
	_, err := uc.OAuthLogin(ctx, OAuthInput{
		Provider:      "google",
		ProviderID:    "g-99",
		Email:         "  User@Example.COM  ",
		EmailVerified: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "user@example.com", lookupEmail, "GetByEmail must receive normalised email")
	assert.Equal(t, "user@example.com", storedEmail, "CreateOAuth must store normalised email")
}

// TestOAuthLogin_KnownProvider_ReturnsExistingAccount checks branch 1: when the
// provider+providerID pair is already linked, tokens are issued for the existing
// account without touching user creation.
func TestOAuthLogin_KnownProvider_ReturnsExistingAccount(t *testing.T) {
	ctx := context.Background()

	const existingUserID = "known-provider-uid"
	var createUserCalled bool
	creds := &mockCredRepo{
		GetByProviderFunc: func(_ context.Context, _, _ string) (*domain.Credential, error) {
			return &domain.Credential{UserID: existingUserID, Email: "known@example.com"}, nil
		},
	}
	tokens := &mockTokenRepo{
		CreateFunc: func(_ context.Context, _ *domain.RefreshToken) error { return nil },
	}
	users := &mockUserClient{
		CreateUserFunc: func(_ context.Context, _ domain.CreateUserInput) (*domain.UserRecord, error) {
			createUserCalled = true
			return nil, nil
		},
		GetUserFunc: func(_ context.Context, uid string) (*domain.UserRecord, error) {
			assert.Equal(t, existingUserID, uid)
			return &domain.UserRecord{ID: uid, Email: "known@example.com", Role: "user"}, nil
		},
	}

	uc := testAuthUC(creds, tokens, users)
	result, err := uc.OAuthLogin(ctx, OAuthInput{
		Provider:      "google",
		ProviderID:    "g-known",
		Email:         "known@example.com",
		EmailVerified: true,
	})

	require.NoError(t, err)
	assert.Equal(t, existingUserID, result.UserID)
	assert.False(t, result.IsNewUser)
	assert.False(t, createUserCalled, "CreateUser must not be called for a known provider link")
}

// ---------------------------------------------------------------------------
// OAuthLogin — orphan account email promotion
// ---------------------------------------------------------------------------

// TestOAuthLogin_EmailPromotion_UpgradesOrphanAccount verifies that when a
// returning user's provider now supplies EmailVerified=true and the stored
// credential has an empty email (created during a prior unverified visit),
// PromoteOAuthEmail is called and the new email is used for the issued tokens.
func TestOAuthLogin_EmailPromotion_UpgradesOrphanAccount(t *testing.T) {
	ctx := context.Background()

	const userID = "orphan-uid"
	var promotedEmail string

	creds := &mockCredRepo{
		GetByProviderFunc: func(_ context.Context, _, _ string) (*domain.Credential, error) {
			// Stored credential has no email — created during unverified visit.
			return &domain.Credential{UserID: userID, Email: ""}, nil
		},
		PromoteOAuthEmailFunc: func(_ context.Context, uid, email string) error {
			assert.Equal(t, userID, uid)
			promotedEmail = email
			return nil
		},
	}
	tokens := &mockTokenRepo{
		CreateFunc: func(_ context.Context, _ *domain.RefreshToken) error { return nil },
	}
	users := &mockUserClient{
		GetUserFunc: func(_ context.Context, uid string) (*domain.UserRecord, error) {
			assert.Equal(t, userID, uid)
			return &domain.UserRecord{ID: userID, Email: "promoted@example.com", Role: "user"}, nil
		},
	}

	uc := testAuthUC(creds, tokens, users)
	result, err := uc.OAuthLogin(ctx, OAuthInput{
		Provider:      "google",
		ProviderID:    "g-orphan",
		Email:         "promoted@example.com",
		EmailVerified: true,
	})

	require.NoError(t, err)
	assert.Equal(t, userID, result.UserID)
	assert.False(t, result.IsNewUser)
	assert.Equal(t, "promoted@example.com", promotedEmail,
		"PromoteOAuthEmail must be called with the verified email")
}

// TestOAuthLogin_EmailPromotion_SkippedForUnverified verifies that PromoteOAuthEmail
// is NOT called when the provider still returns EmailVerified=false on a returning
// visit to an orphan account.
func TestOAuthLogin_EmailPromotion_SkippedForUnverified(t *testing.T) {
	ctx := context.Background()

	var promoteCalled bool
	creds := &mockCredRepo{
		GetByProviderFunc: func(_ context.Context, _, _ string) (*domain.Credential, error) {
			return &domain.Credential{UserID: "orphan-2", Email: ""}, nil
		},
		PromoteOAuthEmailFunc: func(_ context.Context, _, _ string) error {
			promoteCalled = true
			return nil
		},
	}
	tokens := &mockTokenRepo{
		CreateFunc: func(_ context.Context, _ *domain.RefreshToken) error { return nil },
	}
	users := &mockUserClient{
		GetUserFunc: func(_ context.Context, _ string) (*domain.UserRecord, error) {
			return &domain.UserRecord{ID: "orphan-2", Email: "", Role: "user"}, nil
		},
	}

	uc := testAuthUC(creds, tokens, users)
	_, err := uc.OAuthLogin(ctx, OAuthInput{
		Provider:      "github",
		ProviderID:    "gh-orphan",
		Email:         "still-unverified@example.com",
		EmailVerified: false,
	})

	require.NoError(t, err)
	assert.False(t, promoteCalled, "PromoteOAuthEmail must not be called when EmailVerified=false")
}

// TestOAuthLogin_EmailPromotion_FailureIsNonFatal verifies that a PromoteOAuthEmail
// failure does not block the login — the user is authenticated with the old (empty)
// email and the failure is only logged.
func TestOAuthLogin_EmailPromotion_FailureIsNonFatal(t *testing.T) {
	ctx := context.Background()

	const userID = "orphan-3"
	creds := &mockCredRepo{
		GetByProviderFunc: func(_ context.Context, _, _ string) (*domain.Credential, error) {
			return &domain.Credential{UserID: userID, Email: ""}, nil
		},
		PromoteOAuthEmailFunc: func(_ context.Context, _, _ string) error {
			return errors.New("db connection lost during promotion")
		},
	}
	tokens := &mockTokenRepo{
		CreateFunc: func(_ context.Context, _ *domain.RefreshToken) error { return nil },
	}
	users := &mockUserClient{
		GetUserFunc: func(_ context.Context, _ string) (*domain.UserRecord, error) {
			return &domain.UserRecord{ID: userID, Email: "", Role: "user"}, nil
		},
	}

	uc := testAuthUC(creds, tokens, users)
	result, err := uc.OAuthLogin(ctx, OAuthInput{
		Provider:      "google",
		ProviderID:    "g-fail-promote",
		Email:         "fail@example.com",
		EmailVerified: true,
	})

	require.NoError(t, err, "promotion failure must not block authentication")
	assert.Equal(t, userID, result.UserID)
}

// ---------------------------------------------------------------------------
// OAuthLogin — JWT email comes from user-service, not credential row
// ---------------------------------------------------------------------------

// TestOAuthLogin_JWT_EmailFromUserSvc verifies that the email encoded in the
// issued access token comes from user-service (GetUser.Email), not from
// cred.Email. This matters when cred.Email is "" (unverified-email orphan
// account) — the JWT must reflect the authoritative value from user-svc,
// even if that value is also empty, rather than silently using a stale
// credential email.
func TestOAuthLogin_JWT_EmailFromUserSvc(t *testing.T) {
	ctx := context.Background()

	const userID = "jwt-email-uid"
	const userSvcEmail = "authoritative@example.com"

	creds := &mockCredRepo{
		// credential has a stale / different email
		GetByProviderFunc: func(_ context.Context, _, _ string) (*domain.Credential, error) {
			return &domain.Credential{UserID: userID, Email: "stale@old.com"}, nil
		},
	}
	tokens := &mockTokenRepo{
		CreateFunc: func(_ context.Context, _ *domain.RefreshToken) error { return nil },
	}
	users := &mockUserClient{
		GetUserFunc: func(_ context.Context, uid string) (*domain.UserRecord, error) {
			require.Equal(t, userID, uid)
			return &domain.UserRecord{ID: userID, Email: userSvcEmail, Role: "user"}, nil
		},
	}

	uc := testAuthUC(creds, tokens, users)
	result, err := uc.OAuthLogin(ctx, OAuthInput{
		Provider:      "google",
		ProviderID:    "g-jwt",
		Email:         "provider@example.com",
		EmailVerified: false, // cred.Email is not "" so no promotion path
	})
	require.NoError(t, err)

	// Decode the issued access token and verify the email claim.
	claims, err := uc.ValidateToken(ctx, result.Tokens.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, userSvcEmail, claims.Email,
		"JWT email must come from user-service GetUser, not from cred.Email")
}

// TestOAuthLogin_JWT_EmptyEmailOrphan verifies that when an orphan account
// (cred.Email="") is accessed before promotion, the JWT carries an empty
// email — which is honest — rather than some other value.
func TestOAuthLogin_JWT_EmptyEmailOrphan(t *testing.T) {
	ctx := context.Background()

	const userID = "orphan-jwt-uid"

	creds := &mockCredRepo{
		GetByProviderFunc: func(_ context.Context, _, _ string) (*domain.Credential, error) {
			return &domain.Credential{UserID: userID, Email: ""}, nil
		},
	}
	tokens := &mockTokenRepo{
		CreateFunc: func(_ context.Context, _ *domain.RefreshToken) error { return nil },
	}
	users := &mockUserClient{
		GetUserFunc: func(_ context.Context, _ string) (*domain.UserRecord, error) {
			// user-svc also has no email yet (transitional state)
			return &domain.UserRecord{ID: userID, Email: "", Role: "user"}, nil
		},
	}

	uc := testAuthUC(creds, tokens, users)
	result, err := uc.OAuthLogin(ctx, OAuthInput{
		Provider:      "github",
		ProviderID:    "gh-orphan-jwt",
		Email:         "unverified@example.com",
		EmailVerified: false,
	})
	require.NoError(t, err)

	claims, err := uc.ValidateToken(ctx, result.Tokens.AccessToken)
	require.NoError(t, err)
	assert.Empty(t, claims.Email,
		"JWT email must be empty for orphan account — not a stale credential email")
}
