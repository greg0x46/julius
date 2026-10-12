package settings

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"time"

	"github.com/greg0x46/julius/internal/db"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
)

// ErrLocked means that an encryption key was not supplied by the caller.
// It does not represent browser authentication.
var ErrLocked = errors.New("settings: locked")

// Set stores value under key. When encrypted is true, key is a byte
// requiring the externally configured encryption key to encrypt
// it before writing — plaintext config (e.g. sync intervals) should pass
// encrypted=false and a nil key.
func Set(ctx context.Context, q Querier, key, value string, encrypted bool, unlockKey []byte) error {
	stored := value
	isEncrypted := 0
	if encrypted {
		if unlockKey == nil {
			return ErrLocked
		}
		ciphertext, err := encrypt(unlockKey, []byte(value))
		if err != nil {
			return err
		}
		stored = base64.StdEncoding.EncodeToString(ciphertext)
		isEncrypted = 1
	}
	_, err := q.ExecContext(ctx,
		`INSERT INTO settings (key, value, is_encrypted, updated_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT (key) DO UPDATE SET value = excluded.value, is_encrypted = excluded.is_encrypted, updated_at = excluded.updated_at`,
		key, stored, isEncrypted, db.FormatTime(time.Now()),
	)
	return err
}

// Get reads key, decrypting it with unlockKey if it was stored encrypted.
// Returns ok=false when key has no row. Returns ErrLocked when the value is
// encrypted but unlockKey is nil.
func Get(ctx context.Context, q Querier, key string, unlockKey []byte) (value string, ok bool, err error) {
	var stored string
	var isEncrypted int
	err = q.QueryRowContext(ctx, `SELECT value, is_encrypted FROM settings WHERE key = ?`, key).
		Scan(&stored, dbtypes.Bool(&isEncrypted))
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if isEncrypted == 0 {
		return stored, true, nil
	}
	if unlockKey == nil {
		return "", false, ErrLocked
	}
	ciphertext, err := base64.StdEncoding.DecodeString(stored)
	if err != nil {
		return "", false, err
	}
	plaintext, err := decrypt(unlockKey, ciphertext)
	if err != nil {
		return "", false, err
	}
	return string(plaintext), true, nil
}
