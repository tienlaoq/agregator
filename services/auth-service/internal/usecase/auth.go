package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/argon2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
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

type AuthUseCase struct {
	creds      domain.CredentialRepository
	tokens     domain.RefreshTokenRepository
	userClient userv1.UserServiceClient
	jwtSecret  string
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func NewAuthUseCase(
	creds domain.CredentialRepository,
	tokens domain.RefreshTokenRepository,
	userClient userv1.UserServiceClient,
	jwtSecret string,
	accessTTL, refreshTTL time.Duration,
) *AuthUseCase {
	return &AuthUseCase{
		creds:      creds,
		tokens:     tokens,
		userClient: userClient,
		jwtSecret:  jwtSecret,
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
	}
}

type RegisterInput struct {
	Email    string
	Phone    string
	Password string
	Name     string
	Role     string
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

type RegisterResult struct {
	UserID string
	Tokens TokenPair
}

func (uc *AuthUseCase) Register(ctx context.Context, in RegisterInput) (*RegisterResult, error) {
	hash, err := hashPassword(in.Password)
	if err != nil {
		return nil, pkgerr.Internal("failed to hash password")
	}

	userResp, err := uc.userClient.CreateUser(ctx, &userv1.CreateUserRequest{
		Id:    uuid.New().String(),
		Email: in.Email,
		Phone: in.Phone,
		Name:  in.Name,
		Role:  in.Role,
	})
	if err != nil {
		st, ok := status.FromError(err)
		if ok && st.Code() == codes.AlreadyExists {
			return nil, pkgerr.AlreadyExists(st.Message())
		}
		if ok && st.Code() == codes.InvalidArgument {
			return nil, pkgerr.InvalidArgument(st.Message())
		}
		return nil, pkgerr.Internal("ошибка при создании пользователя")
	}

	cred := &domain.Credential{
		UserID:       userResp.Id,
		Email:        in.Email,
		PasswordHash: hash,
	}
	if err := uc.creds.Create(ctx, cred); err != nil {
		return nil, fmt.Errorf("create credential: %w", err)
	}

	tokens, err := uc.generateTokenPair(ctx, userResp.Id, in.Email, in.Role)
	if err != nil {
		return nil, err
	}

	return &RegisterResult{UserID: userResp.Id, Tokens: *tokens}, nil
}

type OAuthInput struct {
	Provider   string
	ProviderID string
	Email      string
	Name       string
	AvatarURL  string
}

type OAuthResult struct {
	UserID    string
	Tokens    TokenPair
	IsNewUser bool
}

func (uc *AuthUseCase) OAuthLogin(ctx context.Context, in OAuthInput) (*OAuthResult, error) {
	cred, err := uc.creds.GetByProvider(ctx, in.Provider, in.ProviderID)
	if err == nil {
		userResp, uerr := uc.userClient.GetUser(ctx, &userv1.GetUserRequest{Id: cred.UserID})
		if uerr != nil {
			return nil, fmt.Errorf("get user: %w", uerr)
		}
		tokens, terr := uc.generateTokenPair(ctx, cred.UserID, cred.Email, userResp.Role)
		if terr != nil {
			return nil, terr
		}
		return &OAuthResult{UserID: cred.UserID, Tokens: *tokens, IsNewUser: false}, nil
	}

	// Try to find existing user by email and link OAuth
	existingCred, err := uc.creds.GetByEmail(ctx, in.Email)
	if err == nil {
		newCred := &domain.Credential{
			UserID:     existingCred.UserID,
			Email:      in.Email,
			Provider:   in.Provider,
			ProviderID: in.ProviderID,
		}
		if cerr := uc.creds.CreateOAuth(ctx, newCred); cerr != nil {
			return nil, fmt.Errorf("link oauth: %w", cerr)
		}
		userResp, uerr := uc.userClient.GetUser(ctx, &userv1.GetUserRequest{Id: existingCred.UserID})
		if uerr != nil {
			return nil, fmt.Errorf("get user: %w", uerr)
		}
		tokens, terr := uc.generateTokenPair(ctx, existingCred.UserID, in.Email, userResp.Role)
		if terr != nil {
			return nil, terr
		}
		return &OAuthResult{UserID: existingCred.UserID, Tokens: *tokens, IsNewUser: false}, nil
	}

	// New user — create via user-service
	userID := uuid.New().String()
	userResp, err := uc.userClient.CreateUser(ctx, &userv1.CreateUserRequest{
		Id:    userID,
		Email: in.Email,
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
		UserID:     userResp.Id,
		Email:      in.Email,
		Provider:   in.Provider,
		ProviderID: in.ProviderID,
	}
	if err := uc.creds.CreateOAuth(ctx, newCred); err != nil {
		return nil, fmt.Errorf("create oauth credential: %w", err)
	}

	tokens, err := uc.generateTokenPair(ctx, userResp.Id, in.Email, "user")
	if err != nil {
		return nil, err
	}
	return &OAuthResult{UserID: userResp.Id, Tokens: *tokens, IsNewUser: true}, nil
}

func (uc *AuthUseCase) Login(ctx context.Context, email, password string) (*RegisterResult, error) {
	cred, err := uc.creds.GetByEmail(ctx, email)
	if err != nil {
		return nil, pkgerr.Unauthenticated("invalid email or password")
	}

	if !verifyPassword(password, cred.PasswordHash) {
		return nil, pkgerr.Unauthenticated("invalid email or password")
	}

	userResp, err := uc.userClient.GetUser(ctx, &userv1.GetUserRequest{Id: cred.UserID})
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	tokens, err := uc.generateTokenPair(ctx, cred.UserID, cred.Email, userResp.Role)
	if err != nil {
		return nil, err
	}

	return &RegisterResult{UserID: cred.UserID, Tokens: *tokens}, nil
}

func (uc *AuthUseCase) RefreshToken(ctx context.Context, rawRefreshToken string) (*TokenPair, error) {
	hash := hashRefreshToken(rawRefreshToken)

	stored, err := uc.tokens.GetByHash(ctx, hash)
	if err != nil {
		return nil, pkgerr.Unauthenticated("invalid refresh token")
	}

	if time.Now().After(stored.ExpiresAt) {
		_ = uc.tokens.DeleteByHash(ctx, hash)
		return nil, pkgerr.Unauthenticated("refresh token expired")
	}

	if err := uc.tokens.DeleteByHash(ctx, hash); err != nil {
		return nil, pkgerr.Internal("failed to revoke old token")
	}

	cred, err := uc.creds.GetByUserID(ctx, stored.UserID)
	if err != nil {
		return nil, pkgerr.Internal("credential not found")
	}

	userResp, err := uc.userClient.GetUser(ctx, &userv1.GetUserRequest{Id: stored.UserID})
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	return uc.generateTokenPair(ctx, stored.UserID, cred.Email, userResp.Role)
}

func (uc *AuthUseCase) ValidateToken(_ context.Context, accessToken string) (*auth.Claims, error) {
	claims, err := auth.ValidateAccessToken(uc.jwtSecret, accessToken)
	if err != nil {
		return nil, pkgerr.Unauthenticated("invalid access token")
	}
	return claims, nil
}

func (uc *AuthUseCase) Logout(ctx context.Context, rawRefreshToken string) error {
	hash := hashRefreshToken(rawRefreshToken)
	return uc.tokens.DeleteByHash(ctx, hash)
}

// --- token helpers ---

func (uc *AuthUseCase) generateTokenPair(ctx context.Context, userID, email, role string) (*TokenPair, error) {
	accessToken, err := auth.GenerateAccessToken(uc.jwtSecret, userID, email, role, uc.accessTTL)
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
