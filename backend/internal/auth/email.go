package auth

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/gomail.v2"
)

type EmailService interface {
	SendVerificationEmail(to, token string) error
	SendPasswordResetEmail(to, token string) error
}

type smtpEmailService struct {
	dialer      *gomail.Dialer
	fromEmail   string
	frontendURL string
}

func NewSMTPEmailService() (EmailService, error) {
	host := os.Getenv("SMTP_HOST")
	user := os.Getenv("SMTP_USER")
	password := os.Getenv("SMTP_PASSWORD")
	frontendURL := os.Getenv("FRONTEND_URL")
	portStr := os.Getenv("SMTP_PORT")
	if portStr == "" || host == "" || user == "" || password == "" || frontendURL == "" {
		return nil, fmt.Errorf("SMTP or FRONTEND_URL environment variables not set")
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("invalid SMTP_PORT: %w", err)
	}

	from := user

	d := gomail.NewDialer(host, port, user, password)

	return &smtpEmailService{
		dialer:      d,
		fromEmail:   from,
		frontendURL: frontendURL,
	}, nil
}

func (s *smtpEmailService) SendVerificationEmail(to, token string) error {
	subject := "Verify Your Email Address"
	verificationLink := fmt.Sprintf("%s/verify-email?token=%s", s.frontendURL, token)
	body := fmt.Sprintf(`
        <p>Hi,</p>
        <p>Thanks for signing up! Please click the link below to verify your email address:</p>
        <p><a href="%s">Verify Email</a></p>
        <p>Thanks,<br>The Team</p>
    `, verificationLink)

	return s.send(to, subject, body)
}

func (s *smtpEmailService) SendPasswordResetEmail(to, token string) error {
	subject := "Reset Your Password"
	resetLink := fmt.Sprintf("%s/reset-password?token=%s", s.frontendURL, token)
	body := fmt.Sprintf(`
        <p>Hi,</p>
        <p>You requested a password reset. Please click the link below to set a new password:</p>
        <p><a href="%s">Reset Password</a></p>
        <p>This link will expire in 1 hour.</p>
        <p>If you did not request this, please ignore this email.</p>
        <p>Thanks,<br>The Team</p>
    `, resetLink)

	return s.send(to, subject, body)
}

func (s *smtpEmailService) send(to, subject, body string) error {
	m := gomail.NewMessage()
	m.SetHeader("From", s.fromEmail)
	m.SetHeader("To", to)
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", body)
	if err := s.dialer.DialAndSend(m); err != nil {
		return fmt.Errorf("could not send email: %w", err)
	}
	return nil
}
