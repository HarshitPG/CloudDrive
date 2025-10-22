package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	"backend/pkg/logger"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

func zapField(key, val string) zap.Field {
	return zap.String(key, val)
}

type Service interface {
	Signup(ctx context.Context, email, password, fullName string) (string, error)
	VerifyEmail(ctx context.Context, token string) error
	Login(ctx context.Context, email, password string) (accessToken string, refreshToken string, err error)
	Refresh(ctx context.Context, refreshToken string) (newAccess string, newRefresh string, err error)
	Logout(ctx context.Context, refreshToken string) error
	ForgotPassword(ctx context.Context, email string) error
	ResetPassword(ctx context.Context, token, newPassword string) error
}

type service struct {
	db         *sql.DB
	jwtSecret  string
	emailSvc   EmailService
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func NewService(db *sql.DB, emailSvc EmailService) Service {
	secret := os.Getenv("JWT_SECRET")
	// if secret == "" {
	// 	secret = "dev-secret"
	// }
	return &service{
		db:         db,
		emailSvc:   emailSvc,
		jwtSecret:  secret,
		accessTTL:  120 * time.Minute,
		refreshTTL: 7 * 24 * time.Hour,
	}
}

func (s *service) Signup(ctx context.Context, email, password, fullName string) (string, error) {
	if email == "" || password == "" {
		return "", errors.New("email and password required")
	}

	var exists bool
	err := s.db.QueryRowContext(ctx, qCheckUserExists, email).Scan(&exists)
	if err != nil {
		return "", err
	}
	if exists {
		return "", fmt.Errorf("user already exists")
	}

	pwhash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	verToken := uuid.NewString()

	_, err = s.db.ExecContext(ctx, qInsertUser, email, fullName, string(pwhash), verToken)
	if err != nil {
		return "", err
	}

	go func() {
		if err := s.emailSvc.SendVerificationEmail(email, verToken); err != nil {
			logger.L.Error("failed to send verification email", zapField("email", email), zap.Error(err))
		}
	}()

	logger.L.Info("user signup", zapField("email", email))

	return verToken, nil
}

func (s *service) VerifyEmail(ctx context.Context, token string) error {
	if token == "" {
		return errors.New("token required")
	}
	res, err := s.db.ExecContext(ctx, qVerifyEmail, token)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("invalid token")
	}
	return nil
}

func (s *service) Login(ctx context.Context, email, password string) (string, string, error) {
	var id string
	var pwhash sql.NullString
	var verified bool
	err := s.db.QueryRowContext(ctx, qGetUserCredentials, email).
		Scan(&id, &pwhash, &verified)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", "", fmt.Errorf("invalid credentials")
		}
		return "", "", err
	}
	if !pwhash.Valid {
		return "", "", fmt.Errorf("account has no password (use SSO)")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(pwhash.String), []byte(password)); err != nil {
		return "", "", fmt.Errorf("invalid credentials")
	}
	if !verified {
		return "", "", fmt.Errorf("email not verified")
	}

	uid, _ := uuid.Parse(id)
	access, err := s.createAccessToken(uid)
	if err != nil {
		return "", "", err
	}
	refresh, err := s.createAndStoreRefreshToken(ctx, uid)
	if err != nil {
		return "", "", err
	}

	logger.L.Info("user.login", zapField("user", id))
	return access, refresh, nil
}

func (s *service) Refresh(ctx context.Context, refreshToken string) (string, string, error) {
	//‼️find matching refresh_tokens row by comparing hashed token
	//‼️For simplicity store plain token hash using bcrypt.CompareHashAndPassword
	//‼️In production,might HMAC + store, or use JWT refresh tokens with jti
	rows, err := s.db.QueryContext(ctx, qGetActiveRefreshTokens)
	if err != nil {
		return "", "", err
	}
	defer rows.Close()

	var foundID string
	var userID string
	var tokenHash string
	var expiresAt time.Time
	var revoked sql.NullTime
	for rows.Next() {
		if err := rows.Scan(&foundID, &userID, &tokenHash, &expiresAt, &revoked); err != nil {
			return "", "", err
		}
		if bcrypt.CompareHashAndPassword([]byte(tokenHash), []byte(refreshToken)) == nil {
			break
		}
		foundID = ""
	}
	if foundID == "" {
		return "", "", fmt.Errorf("invalid refresh token")
	}

	uid, _ := uuid.Parse(userID)
	access, err := s.createAccessToken(uid)
	if err != nil {
		return "", "", err
	}
	newRefresh, err := s.createAndStoreRefreshToken(ctx, uid)
	if err != nil {
		return "", "", err
	}

	_, err = s.db.ExecContext(ctx, qRevokeRefreshToken, foundID)
	if err != nil {
		logger.L.Warn("failed revoke old refresh", zapField("err", err.Error()))
	}

	return access, newRefresh, nil
}

func (s *service) Logout(ctx context.Context, refreshToken string) error {
	rows, err := s.db.QueryContext(ctx, qGetAllRefreshTokens)
	if err != nil {
		return err
	}
	defer rows.Close()
	var foundID string
	var tokenHash string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id, &tokenHash); err != nil {
			return err
		}
		if bcrypt.CompareHashAndPassword([]byte(tokenHash), []byte(refreshToken)) == nil {
			foundID = id
			break
		}
	}
	if foundID != "" {
		_, err := s.db.ExecContext(ctx, qRevokeRefreshToken, foundID)
		return err
	}
	return nil
}

func (s *service) ForgotPassword(ctx context.Context, email string) error {
	if email == "" {
		return errors.New("email required")
	}
	var id string
	err := s.db.QueryRowContext(ctx, qGetUserByEmail, email).Scan(&id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	resetTok := uuid.NewString()
	expires := time.Now().Add(1 * time.Hour)
	_, err = s.db.ExecContext(ctx, qSetResetToken, resetTok, expires, id)
	if err != nil {
		return err
	}
	go func() {
		if err := s.emailSvc.SendPasswordResetEmail(email, resetTok); err != nil {
			logger.L.Error("failed to send password reset email", zapField("email", email), zap.Error(err))
		}
	}()
	logger.L.Info("password.reset_requested", zapField("user", id))
	_ = resetTok
	return nil
}

func (s *service) ResetPassword(ctx context.Context, token, newPassword string) error {
	if token == "" || newPassword == "" {
		return errors.New("token and password required")
	}
	var id string
	var expires sql.NullTime
	err := s.db.QueryRowContext(ctx, qGetResetTokenInfo, token).Scan(&id, &expires)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("invalid token")
		}
		return err
	}
	if !expires.Valid || expires.Time.Before(time.Now()) {
		return fmt.Errorf("token expired")
	}
	pwhash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, qResetPassword, string(pwhash), id)
	if err != nil {
		return err
	}
	logger.L.Info("password.reset", zapField("user", id))
	return nil
}

func (s *service) createAccessToken(uid uuid.UUID) (string, error) {
	claims := jwt.MapClaims{
		"sub": uid.String(),
		"exp": time.Now().Add(s.accessTTL).Unix(),
		"iat": time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	ss, err := token.SignedString([]byte(s.jwtSecret))
	if err != nil {
		return "", err
	}
	return ss, nil
}

func (s *service) createAndStoreRefreshToken(ctx context.Context, uid uuid.UUID) (string, error) {
	plain := uuid.NewString()
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	expires := time.Now().Add(s.refreshTTL)
	_, err = s.db.ExecContext(ctx, qInsertRefreshToken, uid.String(), string(hash), expires)
	if err != nil {
		return "", err
	}
	return plain, nil
}
