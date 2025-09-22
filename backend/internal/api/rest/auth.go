package rest

import (
	"net/http"
	"time"

	"backend/internal/auth"
	"backend/internal/database"
	"backend/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type authHandlers struct {
	authSvc auth.Service
}

func RegisterAuthRoutes(rg *gin.RouterGroup, db database.Service) {

	emailSvc, err := auth.NewSMTPEmailService()
	if err != nil {
		logger.L.Fatal("Failed to initialize email service", zap.Error(err))
	}

	h := &authHandlers{
		authSvc: auth.NewService(db.DB(), emailSvc),
	}

	auth := rg.Group("/auth")
	{
		//	@Summary		Sign up
		//	@Description	Create a new user account and returns a verification token in development
		//	@Tags			auth
		//	@Accept			json
		//	@Produce		json
		//	@Param			body	body		signupRequest	true	"Signup payload"
		//	@Success		201		{object}	signupResponse
		//	@Failure		400		{object}	errorResponse
		//	@Router			/api/v1/auth/signup [post]
		auth.POST("/signup", h.signup)
		//	@Summary	Verify email
		//	@Tags		auth
		//	@Produce	json
		//	@Param		token	query		string	true	"Verification token"
		//	@Success	200		{object}	verifyResponse
		//	@Failure	400		{object}	errorResponse
		//	@Router		/api/v1/auth/verify [get]
		auth.GET("/verify", h.verify) // ?token=<token>
		//	@Summary	Login
		//	@Tags		auth
		//	@Accept		json
		//	@Produce	json
		//	@Param		body	body		loginRequest	true	"Login payload"
		//	@Success	200		{object}	loginResponse
		//	@Failure	400		{object}	errorResponse
		//	@Failure	401		{object}	errorResponse
		//	@Router		/api/v1/auth/login [post]
		auth.POST("/login", h.login)
		auth.POST("/refresh", h.refresh)
		auth.POST("/logout", h.logout)
		auth.POST("/forgot-password", h.forgotPassword)
		auth.POST("/reset-password", h.resetPassword)
	}
}

type signupRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
	FullName string `json:"fullName"`
}

// Auth Response Models
type signupResponse struct {
	Message           string `json:"message" example:"user created"`
	VerificationToken string `json:"verification_token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
}

type verifyResponse struct {
	Message string `json:"message" example:"email verified"`
}

type loginResponse struct {
	AccessToken string `json:"access_token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
}

type refreshResponse struct {
	AccessToken string `json:"access_token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
}

type logoutResponse struct {
	Message string `json:"message" example:"logged out"`
}

type forgotPasswordResponse struct {
	Message string `json:"message" example:"if the email exists, a reset link was sent"`
}

type resetPasswordResponse struct {
	Message string `json:"message" example:"password reset successful"`
}

// Signup godoc
//
//	@Summary		Sign up
//	@Description	Create a new user account and returns a verification token in development
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			body	body		signupRequest	true	"Signup payload"
//	@Success		201		{object}	signupResponse
//	@Failure		400		{object}	errorResponse
//	@Router			/api/v1/auth/signup [post]
func (h *authHandlers) signup(c *gin.Context) {
	var req signupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	token, err := h.authSvc.Signup(c.Request.Context(), req.Email, req.Password, req.FullName)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	//‼️ Return verification token in dev; in prod do not return token, send email instead
	c.JSON(http.StatusCreated, signupResponse{
		Message:           "user created",
		VerificationToken: token,
	})
}

// Verify godoc
//
//	@Summary	Verify email
//	@Tags		auth
//	@Produce	json
//	@Param		token	query		string	true	"Verification token"
//	@Success	200		{object}	verifyResponse
//	@Failure	400		{object}	errorResponse
//	@Router		/api/v1/auth/verify [get]
func (h *authHandlers) verify(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "token required"})
		return
	}
	if err := h.authSvc.VerifyEmail(c.Request.Context(), token); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, verifyResponse{Message: "email verified"})
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// Login godoc
//
//	@Summary	Login
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		body	body		loginRequest	true	"Login payload"
//	@Success	200		{object}	loginResponse
//	@Failure	400		{object}	errorResponse
//	@Failure	401		{object}	errorResponse
//	@Router		/api/v1/auth/login [post]
func (h *authHandlers) login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	access, refresh, err := h.authSvc.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, errorResponse{Error: err.Error()})
		return
	}
	c.SetCookie("refresh_token", refresh, int((7 * 24 * time.Hour).Seconds()), "/", "", false, true)
	c.JSON(http.StatusOK, loginResponse{AccessToken: access})
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// Refresh godoc
//
//	@Summary	Refresh token
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		body	body		refreshRequest	false	"Refresh payload"
//	@Success	200		{object}	refreshResponse
//	@Failure	400		{object}	errorResponse
//	@Failure	401		{object}	errorResponse
//	@Router		/api/v1/auth/refresh [post]
func (h *authHandlers) refresh(c *gin.Context) {
	rt, err := c.Cookie("refresh_token")
	if err != nil {
		var r refreshRequest
		if err := c.ShouldBindJSON(&r); err != nil || r.RefreshToken == "" {
			c.JSON(http.StatusBadRequest, errorResponse{Error: "refresh token required"})
			return
		}
		rt = r.RefreshToken
	}
	newAccess, newRefresh, err := h.authSvc.Refresh(c.Request.Context(), rt)
	if err != nil {
		c.JSON(http.StatusUnauthorized, errorResponse{Error: err.Error()})
		return
	}
	c.SetCookie("refresh_token", newRefresh, int((7 * 24 * time.Hour).Seconds()), "/", "", false, true)
	c.JSON(http.StatusOK, refreshResponse{AccessToken: newAccess})
}

type logoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// Logout godoc
//
//	@Summary	Logout
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		body	body		logoutRequest	false	"Logout payload"
//	@Success	200		{object}	logoutResponse
//	@Router		/api/v1/auth/logout [post]
func (h *authHandlers) logout(c *gin.Context) {
	rt, _ := c.Cookie("refresh_token")
	if rt == "" {
		var r logoutRequest
		_ = c.ShouldBindJSON(&r)
		rt = r.RefreshToken
	}
	if rt != "" {
		_ = h.authSvc.Logout(c.Request.Context(), rt)
		c.SetCookie("refresh_token", "", -1, "/", "", false, true)
	}
	c.JSON(http.StatusOK, logoutResponse{Message: "logged out"})
}

type forgotRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// ForgotPassword godoc
//
//	@Summary	Forgot password
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		body	body		forgotRequest	true	"Email payload"
//	@Success	200		{object}	forgotPasswordResponse
//	@Router		/api/v1/auth/forgot-password [post]
func (h *authHandlers) forgotPassword(c *gin.Context) {
	var req forgotRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	_ = h.authSvc.ForgotPassword(c.Request.Context(), req.Email)
	c.JSON(http.StatusOK, forgotPasswordResponse{Message: "if the email exists, a reset link was sent"})
}

type resetRequest struct {
	Token       string `json:"token" binding:"required"`
	NewPassword string `json:"newPassword" binding:"required,min=6"`
}

// ResetPassword godoc
//
//	@Summary	Reset password
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		body	body		resetRequest	true	"Reset payload"
//	@Success	200		{object}	resetPasswordResponse
//	@Failure	400		{object}	errorResponse
//	@Router		/api/v1/auth/reset-password [post]
func (h *authHandlers) resetPassword(c *gin.Context) {
	var req resetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	if err := h.authSvc.ResetPassword(c.Request.Context(), req.Token, req.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, resetPasswordResponse{Message: "password reset successful"})
}
