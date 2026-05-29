package usecase

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"golang.org/x/crypto/argon2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/tienlao/agregator/pkg/auth"
	pkgerr "github.com/tienlao/agregator/pkg/errors"
	"github.com/tienlao/agregator/services/auth-service/internal/domain"
)

type argon2Params struct {
	time    uint32
	memory  uint32
	threads uint8
	keyLen  uint32
	saltLen uint32
}

var defaultParams = argon2Params{
	time:    1,
	memory:  64 * 1024,
	threads: 4,
	keyLen:  32,
	saltLen: 16,
}

// dummyPasswordHash is computed once at package init and used in Login to run argon2id
// even when the email does not exist in the database. This prevents timing-based
// email enumeration: without it, an absent email returns instantly while a present
// one takes ~64 ms for key-derivation, making the two cases distinguishable.
var dummyPasswordHash string

func init() {
	// Verify that defaultParams itself does not exceed the ceilings enforced by
	// verifyPassword. A misconfigured defaultParams would cause every new hash
	// to be immediately rejected on verification — catch it at startup rather
	// than silently breaking all logins.
	if defaultParams.memory > argon2MaxMemory {
		panic(fmt.Sprintf("auth-service: defaultParams.memory %d KiB exceeds argon2MaxMemory %d KiB", defaultParams.memory, argon2MaxMemory))
	}
	if defaultParams.time > argon2MaxTime {
		panic(fmt.Sprintf("auth-service: defaultParams.time %d exceeds argon2MaxTime %d", defaultParams.time, argon2MaxTime))
	}
	if defaultParams.threads > argon2MaxThreads {
		panic(fmt.Sprintf("auth-service: defaultParams.threads %d exceeds argon2MaxThreads %d", defaultParams.threads, argon2MaxThreads))
	}

	var err error
	dummyPasswordHash, err = hashPassword("dummy-sentinel-password-never-matches")
	if err != nil {
		// hashPassword can only fail on rand.Read; if the OS entropy source is broken
		// at init time we have bigger problems. Panic here to surface the issue early.
		panic("auth-service: failed to pre-compute dummy password hash: " + err.Error())
	}
}

type AuthUseCase struct {
	creds         domain.CredentialRepository
	tokens        domain.RefreshTokenRepository
	resetTokens   domain.PasswordResetRepository
	resetMail     PasswordResetMailer
	resetTTL      time.Duration
	userClient    domain.UserClient
	jwtPrivKey    *ecdsa.PrivateKey // ES256 signing key; never shared with other services
	accessTTL     time.Duration
	refreshTTL    time.Duration
	partnerNotify domain.PartnerNotifier // nil-safe: checked before use
	appLog        zerolog.Logger
}

func NewAuthUseCase(
	creds domain.CredentialRepository,
	tokens domain.RefreshTokenRepository,
	resetTokens domain.PasswordResetRepository,
	resetMail PasswordResetMailer,
	resetTTL time.Duration,
	userClient domain.UserClient,
	jwtPrivKey *ecdsa.PrivateKey,
	accessTTL, refreshTTL time.Duration,
	partnerNotify domain.PartnerNotifier,
	appLog zerolog.Logger,
) *AuthUseCase {
	if resetTTL <= 0 {
		resetTTL = time.Hour
	}
	return &AuthUseCase{
		creds:         creds,
		tokens:        tokens,
		resetTokens:   resetTokens,
		resetMail:     resetMail,
		resetTTL:      resetTTL,
		userClient:    userClient,
		jwtPrivKey:    jwtPrivKey,
		accessTTL:     accessTTL,
		refreshTTL:    refreshTTL,
		partnerNotify: partnerNotify,
		appLog:        appLog,
	}
}

type RegisterInput struct {
	Email    string
	Phone    string
	Password string
	Name     string
	Role     string
}

// selfRegistrationRoles is the exhaustive whitelist of roles a client may request
// during self-registration. Privileged roles (admin, moderator, support, …) must
// be assigned by an admin after the account is created — never via this endpoint.
var selfRegistrationRoles = map[string]struct{}{
	"user":        {},
	"master":      {},
	"venue_owner": {},
}

// validateRegisterInput normalises and validates all fields that arrive from an
// untrusted gRPC caller. It must be called before any IO (hashing, DB, user-svc).
func validateRegisterInput(in *RegisterInput) error {
	// --- email ---
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if in.Email == "" {
		return pkgerr.InvalidArgument("email is required")
	}
	if !strings.Contains(in.Email, "@") {
		return pkgerr.InvalidArgument("email is invalid")
	}

	// --- password ---
	if len(in.Password) < minAccountPasswordLen {
		return pkgerr.InvalidArgument(fmt.Sprintf("password must be at least %d characters", minAccountPasswordLen))
	}

	// --- name ---
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return pkgerr.InvalidArgument("name is required")
	}

	// --- role ---
	// Normalise first so the whitelist check is case-insensitive.
	in.Role = strings.ToLower(strings.TrimSpace(in.Role))
	if in.Role == "" {
		in.Role = "user" // safe default
	}
	if _, allowed := selfRegistrationRoles[in.Role]; !allowed {
		return pkgerr.InvalidArgument("role is not allowed for self-registration")
	}

	return nil
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

type AuthResult struct {
	UserID string
	Tokens TokenPair
}

// Register creates a new account using a two-step saga:
//
//  1. Insert credentials into the local DB (auth-service Postgres).
//     This is the cheapest uniqueness gate: if the email is already taken the
//     INSERT fails immediately with AlreadyExists and nothing in user-service is
//     touched.  Because credentials.user_id has no FK to an external table the
//     insert can happen before user-service knows about the user.
//
//  2. Call user-service.CreateUser.
//     On failure the credential row is removed via a compensating delete so the
//     caller can retry cleanly.  The compensating delete is best-effort: if it
//     also fails we log the orphan and return the original error; the credential
//     row will remain but user-service was never told about the user, so the email
//     is effectively blocked until an operator cleans it up — a far better outcome
//     than the previous behaviour where an orphan in user-service blocked the
//     email permanently with no local record.
func (uc *AuthUseCase) Register(ctx context.Context, in RegisterInput) (*AuthResult, error) {
	if err := validateRegisterInput(&in); err != nil {
		return nil, err
	}

	hash, err := hashPassword(in.Password)
	if err != nil {
		return nil, pkgerr.Internal("failed to hash password")
	}

	// Step 1 — local credential insert (uniqueness gate, cheapest first).
	userID := uuid.New().String()
	cred := &domain.Credential{
		UserID:       userID,
		Email:        in.Email,
		PasswordHash: hash,
	}
	if err := uc.creds.Create(ctx, cred); err != nil {
		st, ok := status.FromError(err)
		if ok && st.Code() == codes.AlreadyExists {
			return nil, pkgerr.AlreadyExists(st.Message())
		}
		// pgx surfaces unique-violation as a plain error without a gRPC status,
		// so also surface it as AlreadyExists for the email UNIQUE constraint.
		if isPgUniqueViolation(err) {
			return nil, pkgerr.AlreadyExists("email already registered")
		}
		return nil, fmt.Errorf("create credential: %w", err)
	}

	// Step 2 — remote user creation.  On failure, compensate by deleting the
	// credential we just inserted so the caller can retry without leaving debris.
	userResp, err := uc.userClient.CreateUser(ctx, domain.CreateUserInput{
		ID:    userID,   // same UUID so the two records stay in sync
		Email: in.Email, // already normalised by validateRegisterInput
		Phone: in.Phone,
		Name:  in.Name,
		Role:  in.Role, // already normalised + whitelisted
	})
	if err != nil {
		// Compensating action: remove the credential so the email is free again.
		if delErr := uc.creds.DeleteByUserID(ctx, userID); delErr != nil {
			uc.appLog.Error().Err(delErr).Str("user_id", userID).
				Msg("register: CreateUser failed and compensating credential delete also failed — orphan credential left in DB")
		}
		st, ok := status.FromError(err)
		if ok && st.Code() == codes.AlreadyExists {
			return nil, pkgerr.AlreadyExists(st.Message())
		}
		if ok && st.Code() == codes.InvalidArgument {
			return nil, pkgerr.InvalidArgument(st.Message())
		}
		return nil, pkgerr.Internal("ошибка при создании пользователя")
	}

	tokens, err := uc.generateTokenPair(ctx, userResp.ID, in.Email, in.Role)
	if err != nil {
		return nil, err
	}

	// Enqueue a partner-registration notification for admin visibility.
	// Enqueue is non-blocking: the TelegramWorker buffers the event and delivers
	// it asynchronously, so the Register response is not delayed by Telegram RTT.
	if (in.Role == "master" || in.Role == "venue_owner") && uc.partnerNotify != nil {
		uc.partnerNotify.Enqueue(domain.PartnerRegisteredEvent{
			Role:   in.Role,
			UserID: userResp.ID,
			Email:  in.Email,
			Phone:  in.Phone,
			Name:   in.Name,
		})
	}

	return &AuthResult{UserID: userResp.ID, Tokens: *tokens}, nil
}

type OAuthInput struct {
	Provider   string
	ProviderID string
	Email      string
	Name       string
	// AvatarURL is received from the OAuth provider and forwarded here by
	// delivery/grpc so it is not silently discarded at the transport boundary.
	// It is NOT currently passed to user-service because CreateUserRequest in
	// user.proto has no avatar_url field; user-service accepts the avatar only
	// via UpdateUser (called by the client after login).
	//
	// TODO: add avatar_url to user.proto CreateUserRequest, regenerate, and
	// pass AvatarURL through CreateUserInput → adapter → user-svc so that new
	// OAuth registrations have their avatar set in one round-trip.
	// See docs/TECH_DEBT.md [AUTH-OAUTH-AVATAR].
	AvatarURL string
	// EmailVerified must be set to true only when the OAuth provider has
	// cryptographically confirmed that the user owns this email address.
	// When false, the email is treated as unverified and must not be used to
	// link to an existing password-based account, preventing account-takeover
	// via a rogue provider that allows unverified email registration.
	EmailVerified bool
}

type OAuthResult struct {
	UserID    string
	Tokens    TokenPair
	IsNewUser bool
}

func (uc *AuthUseCase) OAuthLogin(ctx context.Context, in OAuthInput) (*OAuthResult, error) {
	// Normalise the provider email once, up front, so every branch stores the
	// same canonical form that GetByEmail queries on (lower(trim(email))).
	// This keeps the stored value consistent with the functional unique index
	// added in migration 005 and with the normalisation done in validateRegisterInput.
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))

	// --- Branch 1: provider already linked to an account ---
	//
	// Email promotion: if the credential was created when the provider did not
	// supply a verified email (email == ""), and NOW the provider confirms
	// EmailVerified=true, we backfill the email on the credential row. This
	// converts an orphan account (no email, no password) into a full account
	// without requiring re-registration. The update is best-effort: a failure
	// is logged but does not block the login.
	//
	// Note: user-service is NOT updated here — that would require an UpdateUser
	// RPC that does not yet exist. The credential email is used for JWT claims
	// and GetByEmail lookups within auth-service only; user-service stores its
	// own copy of the email independently. See docs/TECH_DEBT.md [AUTH-OAUTH-ORPHAN].
	cred, err := uc.creds.GetByProvider(ctx, in.Provider, in.ProviderID)
	if err == nil {
		if cred.Email == "" && in.EmailVerified && in.Email != "" {
			if promoteErr := uc.creds.PromoteOAuthEmail(ctx, cred.UserID, in.Email); promoteErr != nil {
				uc.appLog.Warn().Err(promoteErr).
					Str("user_id", cred.UserID).Str("provider", in.Provider).
					Msg("oauth: email promotion failed — continuing with empty email")
			} else {
				uc.appLog.Info().
					Str("user_id", cred.UserID).Str("provider", in.Provider).
					Msg("oauth: promoted unverified-email account to verified email")
				cred.Email = in.Email
			}
		}
		return uc.issueOAuthTokens(ctx, cred.UserID, false)
	}

	// --- Branch 2: link OAuth to existing account by verified email ---
	//
	// Security: linking an unverified email to an existing account is an
	// account-takeover vector. An attacker can register with any OAuth provider
	// that does not require email verification, use victim@example.com as their
	// provider email, and arrive here with in.Email == victim's address. If we
	// linked without EmailVerified=true they would inherit the victim's account.
	//
	// When EmailVerified is false we fall through to new-user creation below.
	// The new account will have no email set (empty string passed to user-svc)
	// so it cannot collide with the victim's record, and the attacker gains
	// nothing beyond an isolated, email-less account.
	if in.EmailVerified {
		existingCred, err := uc.creds.GetByEmail(ctx, in.Email)
		if err == nil {
			newCred := &domain.Credential{
				UserID:     existingCred.UserID,
				Email:      in.Email, // already normalised above
				Provider:   in.Provider,
				ProviderID: in.ProviderID,
			}
			if cerr := uc.creds.CreateOAuth(ctx, newCred); cerr != nil {
				// Concurrent request already linked this provider → find it and issue tokens.
				// The UNIQUE index on (provider, provider_id) guarantees the row now exists.
				if isPgUniqueViolation(cerr) || isAlreadyExists(cerr) {
					linked, ferr := uc.creds.GetByProvider(ctx, in.Provider, in.ProviderID)
					if ferr != nil {
						return nil, fmt.Errorf("link oauth concurrent race: %w", ferr)
					}
					return uc.issueOAuthTokens(ctx, linked.UserID, false)
				}
				return nil, fmt.Errorf("link oauth: %w", cerr)
			}
			return uc.issueOAuthTokens(ctx, existingCred.UserID, false)
		}
	} else {
		uc.appLog.Warn().Str("provider", in.Provider).Str("provider_id", in.ProviderID).
			Msg("oauth: email_verified=false — skipping email-based account linking to prevent takeover")
	}

	// --- Branch 3: new user ---
	// If the email was not verified by the provider we do not pass it to user-svc:
	// storing an unverified email would silently claim the address and prevent the
	// real owner from ever registering or resetting their password.
	// in.Email is already normalised; use it directly when verified.
	registrationEmail := in.Email
	if !in.EmailVerified {
		registrationEmail = ""
	}
	userID := uuid.New().String()
	userResp, err := uc.userClient.CreateUser(ctx, domain.CreateUserInput{
		ID:    userID,
		Email: registrationEmail,
		Name:  in.Name,
		Role:  "user",
	})
	if err != nil {
		st, ok := status.FromError(err)
		if ok && st.Code() == codes.AlreadyExists {
			return nil, pkgerr.AlreadyExists(st.Message())
		}
		return nil, pkgerr.Internal("ошибка при создании пользователя")
	}

	newCred := &domain.Credential{
		UserID:     userResp.ID,
		Email:      registrationEmail,
		Provider:   in.Provider,
		ProviderID: in.ProviderID,
	}
	if err := uc.creds.CreateOAuth(ctx, newCred); err != nil {
		// Concurrent request with the same provider/providerID already inserted a
		// credential row (UNIQUE idx_credentials_provider). The other goroutine's
		// CreateUser call in user-service produced a different userID — that record
		// is now an orphan in user-service (no credential on our side points to it).
		// We cannot delete it here without an UpdateUser/DeleteUser RPC that does
		// not yet exist; see docs/TECH_DEBT.md [AUTH-OAUTH-RACE-ORPHAN].
		// The correct recovery is to serve the winner's credential.
		if isPgUniqueViolation(err) || isAlreadyExists(err) {
			uc.appLog.Warn().
				Str("provider", in.Provider).Str("provider_id", in.ProviderID).
				Str("orphan_user_id", userResp.ID).
				Msg("oauth: concurrent new-user race — credential already inserted by peer; " +
					"orphan user-service record may exist for orphan_user_id")
			existing, ferr := uc.creds.GetByProvider(ctx, in.Provider, in.ProviderID)
			if ferr != nil {
				return nil, fmt.Errorf("oauth concurrent race recovery: %w", ferr)
			}
			return uc.issueOAuthTokens(ctx, existing.UserID, false)
		}
		return nil, fmt.Errorf("create oauth credential: %w", err)
	}

	// New user: role is always "user" and is already known — no extra GetUser call needed.
	tokens, err := uc.generateTokenPair(ctx, userResp.ID, registrationEmail, "user")
	if err != nil {
		return nil, err
	}
	return &OAuthResult{UserID: userResp.ID, Tokens: *tokens, IsNewUser: true}, nil
}

// issueOAuthTokens fetches the current role and email from user-service and
// issues a fresh token pair for an existing account. Used by the two "returning
// user" branches of OAuthLogin (provider-linked and email-linked) to avoid
// repeating the GetUser → generateTokenPair → OAuthResult pattern.
//
// Email source: user-service is the source of truth for the email that goes
// into the JWT claims. The caller no longer passes an email parameter —
// using cred.Email would embed a potentially stale or empty value (e.g. when
// the credential was created during an unverified-email OAuth flow). If
// user-service returns an empty email (transitional state before an
// UpdateEmail RPC is available, see docs/TECH_DEBT.md [AUTH-OAUTH-ORPHAN]),
// the JWT will carry an empty email — which is at least honest rather than
// silently embedding a stale credential email.
func (uc *AuthUseCase) issueOAuthTokens(ctx context.Context, userID string, isNewUser bool) (*OAuthResult, error) {
	userResp, err := uc.userClient.GetUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	tokens, err := uc.generateTokenPair(ctx, userID, userResp.Email, userResp.Role)
	if err != nil {
		return nil, err
	}
	return &OAuthResult{UserID: userID, Tokens: *tokens, IsNewUser: isNewUser}, nil
}

func (uc *AuthUseCase) Login(ctx context.Context, email, password string) (*AuthResult, error) {
	// Normalise before lookup so the parameter matches the functional index
	// lower(trim(email)) used in GetByEmail (migration 005).
	email = strings.ToLower(strings.TrimSpace(email))

	cred, credErr := uc.creds.GetByEmail(ctx, email)

	// Always run argon2id regardless of whether the email exists or has a password.
	// This keeps response time constant and prevents timing-based email enumeration.
	//
	// hashToVerify selection:
	//   - email not found (credErr != nil)   → dummyPasswordHash (never matches)
	//   - found but no password hash         → dummyPasswordHash (never matches)
	//     This is the OAuth-only case: the credential exists but was created via
	//     OAuthLogin and has no password set. We must not skip argon2id here —
	//     the timing difference between "no such user" and "OAuth-only user" would
	//     leak account existence even if both ultimately return Unauthenticated.
	//   - found with a password hash         → cred.PasswordHash
	hashToVerify := dummyPasswordHash
	if credErr == nil && accountCredentialHasPassword(cred) {
		hashToVerify = cred.PasswordHash
	}
	passwordOK := verifyPassword(password, hashToVerify)

	if credErr != nil || !passwordOK {
		return nil, pkgerr.Unauthenticated("invalid email or password")
	}

	// Explicit guard: an OAuth-only credential (PasswordHash == "") must never
	// reach this point. verifyPassword already returns false for an empty hash,
	// but the check here makes the invariant readable and prevents a regression
	// if verifyPassword is ever changed.
	if !accountCredentialHasPassword(cred) {
		return nil, pkgerr.Unauthenticated("invalid email or password")
	}

	userResp, err := uc.userClient.GetUser(ctx, cred.UserID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	tokens, err := uc.generateTokenPair(ctx, cred.UserID, cred.Email, userResp.Role)
	if err != nil {
		return nil, err
	}

	return &AuthResult{UserID: cred.UserID, Tokens: *tokens}, nil
}

// RefreshToken rotates a refresh token: atomically revokes the old one, then
// fetches fresh identity data and issues a new pair.
//
// # Operation order and security properties
//
// Step 1 — ConsumeByHash (DELETE … RETURNING).
// The token is deleted before any identity data is fetched. This has two
// consequences:
//
//   - No userID leak: a caller with a bogus or expired hash learns nothing about
//     whether that userID exists in the system, because we never touch user-svc
//     or credentials for an unrecognised token.
//   - Concurrent-replay safety: if two requests arrive simultaneously with the
//     same raw token, exactly one DELETE wins; the other receives NotFound and
//     returns Unauthenticated immediately. No window for double-use.
//
// Step 2 — expiry check on the returned row.
// The DELETE unconditionally removes the row regardless of expiry. We check
// ExpiresAt on the returned record and, if it has elapsed, treat it as invalid.
// The row is already gone, so no further cleanup is needed.
//
// Step 3 — fetch identity (credentials + user-svc) after the token is consumed.
// If either call fails the token is already gone. The client must re-authenticate.
// This is the accepted trade-off for avoiding the pre-fetch userID leak; it is
// the same failure mode as any stateless token architecture where the signing
// key is unavailable.
//
// # Reuse-detection limitation
//
// ConsumeByHash catches the concurrent-request race (two overlapping refreshes),
// but NOT a delayed replay: if an attacker stole the token and used it before the
// legitimate owner, the owner's subsequent attempt finds the row already gone and
// receives a plain Unauthenticated — identical to an expired token. The attacker's
// newly issued pair is not revoked. Full family-invalidation on delayed replay
// requires a tombstone table keyed by token hash with TTL == refreshTTL so that
// "was this hash ever consumed?" can be answered after the row is deleted.
// See docs/TECH_DEBT.md [AUTH-REUSE-TOMBSTONE].
//
// # Retry hazard
//
// ConsumeByHash is NOT idempotent. If a client sends a RefreshToken request,
// the server consumes the old token and issues a new pair, but the response is
// lost in transit (network timeout, connection reset), and the client retries
// with the same raw token — the retry receives Unauthenticated because the row
// was already deleted. The client is forced to full re-authentication even
// though the server successfully processed the first request.
//
// This is an inherent tension between DELETE-first atomicity (no userID leak)
// and at-least-once delivery guarantees. Mitigation options:
//
//  1. Idempotency via tombstone: persist {consumed_token_hash → new_token_hash}
//     with TTL == refreshTTL in `consumed_refresh_tokens`. On NotFound, check the
//     tombstone; if the hash is present and consumed_at > now()-graceWindow (e.g.
//     30 s), re-return the stored new token hash. This is safe only if the new
//     token is stored before the old one is deleted (transaction).
//     See docs/TECH_DEBT.md [AUTH-REFRESH-RETRY].
//
//  2. Client-side: implement exponential back-off with a short (≤ 5 s) initial
//     timeout on the refresh call; store the new token pair immediately on
//     receipt before any other network activity. Do NOT retry a refresh that
//     timed out — treat it as a forced re-login. This costs nothing server-side
//     and is sufficient for most mobile clients.
//
// Current posture: no server-side retry protection. Clients must follow option 2.
// See docs/TECH_DEBT.md [AUTH-REFRESH-RETRY].
func (uc *AuthUseCase) RefreshToken(ctx context.Context, rawRefreshToken string) (*TokenPair, error) {
	hash := hashRefreshToken(rawRefreshToken)

	// --- 1. Atomically consume the token ---
	// DELETE … RETURNING: either we get the row and own it, or it doesn't exist.
	// No separate SELECT means no window between "does it exist?" and "delete it".
	stored, err := uc.tokens.ConsumeByHash(ctx, hash)
	if err != nil {
		if isNotFound(err) {
			return nil, pkgerr.Unauthenticated("invalid or already used refresh token")
		}
		return nil, pkgerr.Internal("failed to consume refresh token")
	}

	// --- 2. Reject expired tokens (row is already deleted above) ---
	if time.Now().After(stored.ExpiresAt) {
		return nil, pkgerr.Unauthenticated("refresh token expired")
	}

	// --- 3. Fetch identity ---
	// Token is already gone; if either call fails the client must re-authenticate.
	cred, err := uc.creds.GetByUserID(ctx, stored.UserID)
	if err != nil {
		return nil, pkgerr.Internal("credential not found")
	}

	userResp, err := uc.userClient.GetUser(ctx, stored.UserID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	// --- 4. Issue new token pair ---
	return uc.generateTokenPair(ctx, stored.UserID, cred.Email, userResp.Role)
}

// ValidateToken verifies an ES256 access token and returns its claims.
//
// The context is accepted for interface consistency but intentionally unused:
// ES256 verification is a pure in-process operation (ECDSA signature check +
// exp/nbf comparison) with no I/O and no cancellation points. There is nothing
// to cancel and no deadline to enforce.
//
// If a Redis denylist is ever added (see docs/TECH_DEBT.md [AUTH-DENYLIST]),
// the context must be threaded through to the cache lookup at that point.
func (uc *AuthUseCase) ValidateToken(_ context.Context, accessToken string) (*auth.Claims, error) {
	claims, err := auth.ValidateAccessToken(&uc.jwtPrivKey.PublicKey, accessToken)
	if err != nil {
		return nil, pkgerr.Unauthenticated("invalid access token")
	}
	return claims, nil
}

// Logout revokes the supplied refresh token so it cannot be used to obtain new
// access tokens.  The corresponding access token is intentionally NOT
// invalidated — this is a deliberate design trade-off:
//
// Access tokens are short-lived (default: 15 min, see JWT_ACCESS_TTL).  They
// are stateless signed JWTs verified at the gateway without any DB or cache
// lookup, which is the main scalability and latency advantage of the current
// architecture.  Maintaining a denylist would require every request to hit
// Redis, adding a network round-trip and a new operational dependency on the
// hot path.
//
// Accepted residual risk: after logout the old access token remains valid until
// its exp claim elapses.  The window is bounded by accessTTL (≤ 15 min).
// Clients SHOULD discard the access token on logout so it never leaves the
// browser/app, which limits the practical risk to token theft scenarios
// (addressed separately by HTTPS + secure/httpOnly cookie storage).
//
// If immediate access-token invalidation ever becomes a requirement (e.g. for
// high-privilege admin sessions or HIPAA/PCI compliance), add a Redis-backed
// denylist keyed by the JWT `jti` claim with TTL == remaining exp.  The gateway
// already connects to Redis (REDIS_ADDR); the auth package's Claims struct
// would need a Jti field, and the gateway middleware would need one GET per
// request.  See docs/TECH_DEBT.md [AUTH-DENYLIST] when that work is scoped.
func (uc *AuthUseCase) Logout(ctx context.Context, rawRefreshToken string) error {
	hash := hashRefreshToken(rawRefreshToken)
	err := uc.tokens.DeleteByHash(ctx, hash)
	// NotFound is fine — the token may have already expired or been revoked.
	// Logout is idempotent: don't penalise the client for calling it twice.
	if isNotFound(err) {
		return nil
	}
	return err
}

// --- token helpers ---

func (uc *AuthUseCase) generateTokenPair(ctx context.Context, userID, email, role string) (*TokenPair, error) {
	accessToken, err := auth.GenerateAccessToken(uc.jwtPrivKey, userID, email, role, uc.accessTTL)
	if err != nil {
		return nil, pkgerr.Internal("failed to generate access token")
	}

	rawRefresh, err := generateRefreshToken()
	if err != nil {
		return nil, pkgerr.Internal("failed to generate refresh token")
	}

	rt := &domain.RefreshToken{
		UserID:    userID,
		TokenHash: hashRefreshToken(rawRefresh),
		ExpiresAt: time.Now().Add(uc.refreshTTL),
	}
	if err := uc.tokens.Create(ctx, rt); err != nil {
		return nil, fmt.Errorf("store refresh token: %w", err)
	}

	return &TokenPair{AccessToken: accessToken, RefreshToken: rawRefresh}, nil
}

func generateRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// hashRefreshToken hashes an opaque raw secret for storage (refresh tokens and password-reset links).
func hashRefreshToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// --- argon2id password helpers ---

func hashPassword(password string) (string, error) {
	salt := make([]byte, defaultParams.saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	key := argon2.IDKey(
		[]byte(password), salt,
		defaultParams.time, defaultParams.memory, defaultParams.threads, defaultParams.keyLen,
	)

	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		defaultParams.memory, defaultParams.time, defaultParams.threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// argon2Limits caps the resource parameters parsed from a stored hash.
// These are defence-in-depth bounds: even if an adversary manages to write a
// crafted hash string into the DB (e.g. via a future vulnerability), a single
// Login call cannot consume more than these resources.
//
// The ceilings are deliberately generous relative to defaultParams so that
// legitimate future parameter upgrades do not require a code change, while
// still preventing pathological values (4 GiB RAM, hundreds of iterations).
//
//	mem  ≤ 256 MiB  (defaultParams uses 64 MiB; 4× headroom for future upgrades)
//	iter ≤ 10       (defaultParams uses 1;  10× headroom)
//	par  ≤ 8        (defaultParams uses 4;  2× headroom; also caps goroutine fan-out)
const (
	argon2MaxMemory  uint32 = 256 * 1024 // KiB → 256 MiB
	argon2MaxTime    uint32 = 10
	argon2MaxThreads uint8  = 8
)

func verifyPassword(password, encodedHash string) bool {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return false
	}

	var mem, iter uint32
	var par uint8
	_, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mem, &iter, &par)
	if err != nil {
		return false
	}

	// Clamp parsed parameters to their ceilings before allocating anything.
	// A value of 0 for memory or iterations would make argon2.IDKey panic or
	// produce a trivially weak derivation, so treat it as invalid as well.
	if mem == 0 || mem > argon2MaxMemory {
		return false
	}
	if iter == 0 || iter > argon2MaxTime {
		return false
	}
	if par == 0 || par > argon2MaxThreads {
		return false
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}

	computed := argon2.IDKey([]byte(password), salt, iter, mem, par, uint32(len(expectedHash)))
	return subtle.ConstantTimeCompare(computed, expectedHash) == 1
}

// isNotFound checks if an error is a gRPC NotFound status.
func isNotFound(err error) bool {
	s, ok := status.FromError(err)
	return ok && s.Code() == codes.NotFound
}

// isAlreadyExists checks if an error is a gRPC AlreadyExists status.
// Used alongside isPgUniqueViolation to handle uniqueness errors regardless
// of whether the repository layer has already translated them to gRPC codes.
func isAlreadyExists(err error) bool {
	s, ok := status.FromError(err)
	return ok && s.Code() == codes.AlreadyExists
}

// isPgUniqueViolation reports whether err is a PostgreSQL unique-constraint
// violation (SQLSTATE 23505). pgx does not wrap these as gRPC statuses, so we
// inspect the error message as a fallback when the repository layer does not
// translate them.
func isPgUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	// pgconn.PgError carries the SQLState field; we use errors.As so this works
	// even when the error is wrapped.
	type pgErr interface{ SQLState() string }
	var pe pgErr
	if errors.As(err, &pe) {
		return pe.SQLState() == "23505"
	}
	return false
}
