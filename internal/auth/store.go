package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/settings"
)

var ErrCredentials = errors.New("e-mail ou senha incorretos")
var ErrSession = errors.New("sessão ausente ou expirada")

const Lifetime = 7 * 24 * time.Hour
const IdleTimeout = 24 * time.Hour

type Store struct {
	DB  *sql.DB
	Now func() time.Time
}

func NewStore(db *sql.DB) *Store { return &Store{DB: db, Now: time.Now} }
func (s *Store) Ready(ctx context.Context, master []byte) error {
	var id int
	if err := s.DB.QueryRowContext(ctx, `SELECT 1 FROM auth_owner WHERE id = 1`).Scan(&id); err != nil {
		return fmt.Errorf("autenticação pendente: execute auth init ou auth migrate: %w", err)
	}
	return settings.VerifyMaster(ctx, s.DB, master)
}

// Initialize creates the owner and migrates secrets atomically, with the server stopped.
func (s *Store) Initialize(ctx context.Context, email, password string, master []byte, legacyPassword *string) error {
	email = NormalizeEmail(email)
	if err := validateCredentials(email, password); err != nil {
		return err
	}
	if len(master) != 32 {
		return fmt.Errorf("chave inválida")
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM auth_owner`).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return fmt.Errorf("autenticação já configurada")
	}
	configured, err := settings.IsConfigured(ctx, tx)
	if err != nil {
		return err
	}
	if configured != (legacyPassword != nil) {
		return fmt.Errorf("use auth migrate para banco legado e auth init para banco novo")
	}
	if legacyPassword != nil {
		err = settings.MigrateEncryption(ctx, tx, *legacyPassword, master)
	} else {
		err = settings.InitializeMaster(ctx, tx, master)
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO auth_owner(id,email,password_hash,created_at) VALUES(1,?,?,?)`, email, hash, db.FormatTime(s.Now().Truncate(time.Second))); err != nil {
		return err
	}
	return tx.Commit()
}
func digest(token string) string { h := sha256.Sum256([]byte(token)); return hex.EncodeToString(h[:]) }
func (s *Store) Login(ctx context.Context, email, password string) (string, error) {
	var storedEmail, hash string
	if err := s.DB.QueryRowContext(ctx, `SELECT email,password_hash FROM auth_owner WHERE id=1`).Scan(&storedEmail, &hash); err != nil {
		return "", err
	}
	// Perform the same KDF even when the email is wrong. Request body is bounded by HTTP.
	valid := checkPassword(password, hash)
	if !valid || NormalizeEmail(email) != storedEmail {
		return "", ErrCredentials
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	now := s.Now().Truncate(time.Second)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM auth_sessions WHERE expires_at <= ? OR last_seen_at <= ?`, db.FormatTime(now), db.FormatTime(now.Add(-IdleTimeout))); err != nil {
		return "", err
	}
	// Compare the verified hash so a concurrent reset cannot issue an old-password session.
	result, err := tx.ExecContext(ctx, `INSERT INTO auth_sessions(token_hash,owner_id,revision,created_at,last_seen_at,expires_at) SELECT ?,id,revision,?,?,? FROM auth_owner WHERE id=1 AND password_hash=?`, digest(token), db.FormatTime(now), db.FormatTime(now), db.FormatTime(now.Add(Lifetime)), hash)
	if err != nil {
		return "", err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return "", err
	}
	if n != 1 {
		return "", ErrCredentials
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	return token, nil
}
func (s *Store) Authenticate(ctx context.Context, token string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return "", ErrSession
	}
	now := s.Now().Truncate(time.Second)
	result, err := s.DB.ExecContext(ctx, `UPDATE auth_sessions SET last_seen_at=? WHERE token_hash=? AND expires_at>? AND last_seen_at>? AND revision=(SELECT revision FROM auth_owner WHERE id=1)`, db.FormatTime(now), digest(token), db.FormatTime(now), db.FormatTime(now.Add(-IdleTimeout)))
	if err != nil {
		return "", err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return "", err
	}
	if n != 1 {
		return "", ErrSession
	}
	var email string
	err = s.DB.QueryRowContext(ctx, `SELECT email FROM auth_owner WHERE id=1`).Scan(&email)
	return email, err
}
func (s *Store) Logout(ctx context.Context, token string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM auth_sessions WHERE token_hash=?`, digest(token))
	return err
}

// ResetPassword requires server access; ChangePassword additionally verifies the old password.
func (s *Store) ResetPassword(ctx context.Context, password string) error {
	return s.replacePassword(ctx, nil, password)
}
func (s *Store) ChangePassword(ctx context.Context, old, password string) error {
	return s.replacePassword(ctx, &old, password)
}
func (s *Store) replacePassword(ctx context.Context, old *string, password string) error {
	if !ValidPassword(password) {
		return fmt.Errorf("a senha não pode ficar vazia")
	}
	var previous string
	if err := s.DB.QueryRowContext(ctx, `SELECT password_hash FROM auth_owner WHERE id=1`).Scan(&previous); err != nil {
		return err
	}
	if old != nil && !checkPassword(*old, previous) {
		return ErrCredentials
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE auth_owner SET password_hash=?, revision=revision+1 WHERE id=1 AND password_hash=?`, hash, previous)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrCredentials
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM auth_sessions`); err != nil {
		return err
	}
	return tx.Commit()
}
