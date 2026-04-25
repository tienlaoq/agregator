package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"golang.org/x/crypto/argon2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	"github.com/tienlao/agregator/pkg/auth"
	pkgerr "github.com/tienlao/agregator/pkg/errors"
	pkgtelegram "github.com/tienlao/agregator/pkg/telegram"
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
	creds       domain.CredentialRepository
	tokens      domain.RefreshTokenRepository
	userClient  userv1.UserServiceClient
	jwtSecret   string
	accessTTL   time.Duration
	refreshTTL  time.Duration
	tg          *pkgtelegram.Client
	frontendURL string
	appLog      zerolog.Logger
}

func NewAuthUseCase(
	creds domain.CredentialRepository,
	tokens domain.RefreshTokenRepository,
	userClient userv1.UserServiceClient,
	jwtSecret string,
	accessTTL, refreshTTL time.Duration,
	tg *pkgtelegram.Client,
	frontendURL string,
	appLog zerolog.Logger,
) *AuthUseCase {
	return &AuthUseCase{
		creds:       creds,
		tokens:      tokens,
		userClient:  userClient,
		jwtSecret:   jwtSecret,
		accessTTL:   accessTTL,
		refreshTTL:  refreshTTL,
		tg:          tg,
		frontendURL: frontendURL,
		appLog:      appLog,
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

	roleKey := strings.TrimSpace(strings.ToLower(in.Role))
	if roleKey == "master" || roleKey == "venue_owner" {
		uid := userResp.Id
		email := in.Email
		phone := in.Phone
		name := in.Name
		go uc.notifyPartnerRegistered(roleKey, uid, email, phone, name)
	}

	return &RegisterResult{UserID: userResp.Id, Tokens: *tokens}, nil
}

func (uc *AuthUseCase) notifyPartnerRegistered(role, userID, email, phone, name string) {
	if uc.tg == nil || !uc.tg.Enabled() {
		return
	}
	var roleRu, adminPath, emoji string
	switch role {
	case "master":
		roleRu, adminPath, emoji = "Пар-мастер", "/admin/masters", "🧖"
	case "venue_owner":
		roleRu, adminPath, emoji = "Партнёр бани", "/admin/venues", "🏛"
	default:
		return
	}
	base := strings.TrimSuffix(strings.TrimSpace(uc.frontendURL), "/")
	if base == "" {
		base = "http://localhost:3000"
	}
	link := base + adminPath
	phoneDisp := strings.TrimSpace(phone)
	if phoneDisp == "" {
		phoneDisp = "—"
	}
	text := fmt.Sprintf(
		"%s <b>Новая регистрация</b>\n\n"+
			"Тип аккаунта: <b>%s</b>\n"+
			"Имя: %s\n"+
			"Email: <code>%s</code>\n"+
			"Телефон: %s\n"+
			"User ID: <code>%s</code>\n\n"+
			`<a href="%s">Открыть в админке</a>`,
		emoji,
		html.EscapeString(roleRu),
		html.EscapeString(strings.TrimSpace(name)),
		html.EscapeString(strings.TrimSpace(email)),
		html.EscapeString(phoneDisp),
		html.EscapeString(userID),
		html.EscapeString(link),
	)
	if err := uc.tg.SendHTML(text); err != nil {
		uc.appLog.Warn().Err(err).Str("role", role).Str("user_id", userID).Msg("telegram partner registration failed")
	}
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
