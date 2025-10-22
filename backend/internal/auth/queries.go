package auth

const (
	// User existence check
	qCheckUserExists = `SELECT EXISTS(SELECT 1 FROM users WHERE email=$1)`

	// User creation
	qInsertUser = `
		INSERT INTO users (email, full_name, password_hash, verification_token, is_email_verified, created_at, updated_at)
		VALUES ($1,$2,$3,$4,false,now(),now())`

	// Email verification
	qVerifyEmail = `
		UPDATE users SET is_email_verified = true, verification_token = NULL, updated_at = now()
		WHERE verification_token = $1`

	// Login queries
	qGetUserCredentials = `SELECT id,password_hash,is_email_verified FROM users WHERE email=$1`

	// Refresh token queries
	qGetActiveRefreshTokens = `
		SELECT id, user_id, token_hash, expires_at, revoked_at 
		FROM refresh_tokens 
		WHERE revoked_at IS NULL AND expires_at > now()`

	qInsertRefreshToken = `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at, created_at) 
		VALUES ($1,$2,$3,now())`

	qRevokeRefreshToken = `UPDATE refresh_tokens SET revoked_at=now() WHERE id=$1`

	qGetAllRefreshTokens = `SELECT id, token_hash FROM refresh_tokens WHERE revoked_at IS NULL`

	// Password reset queries
	qGetUserByEmail = `SELECT id FROM users WHERE email=$1`

	qSetResetToken = `
		UPDATE users 
		SET reset_password_token=$1, reset_password_expires_at=$2 
		WHERE id=$3`

	qGetResetTokenInfo = `
		SELECT id, reset_password_expires_at 
		FROM users 
		WHERE reset_password_token=$1`

	qResetPassword = `
		UPDATE users 
		SET password_hash=$1, reset_password_token=NULL, reset_password_expires_at=NULL 
		WHERE id=$2`

	// Admin role check
	qCheckAdminRole = `SELECT is_admin FROM users WHERE id=$1`
)
