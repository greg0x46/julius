package settings

import (
	"context"
	"fmt"
)

const masterVerifierKey = "crypto.master_verifier"
const masterVerifierValue = "contadinho-master-key-v1"

// InitializeMaster records an authenticated check even before Pluggy is configured.
func InitializeMaster(ctx context.Context, q Querier, key []byte) error {
	return Set(ctx, q, masterVerifierKey, masterVerifierValue, true, key)
}
func VerifyMaster(ctx context.Context, q Querier, key []byte) error {
	value, ok, err := Get(ctx, q, masterVerifierKey, key)
	if err != nil || !ok || value != masterVerifierValue {
		return fmt.Errorf("chave de criptografia inválida ou migração pendente")
	}
	// Fail before starting the worker if any encrypted setting is damaged.
	rows, err := q.QueryContext(ctx, `SELECT key FROM settings WHERE is_encrypted = TRUE`)
	if err != nil {
		return err
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		names = append(names, name)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, name := range names {
		if _, _, err := Get(ctx, q, name, key); err != nil {
			return fmt.Errorf("configuração criptografada inválida: %w", err)
		}
	}
	return nil
}

// MigrateEncryption must be called inside the owner's initialization transaction.
func MigrateEncryption(ctx context.Context, q Querier, password string, master []byte) error {
	old, err := VerifyPassword(ctx, q, password)
	if err != nil {
		return err
	}
	rows, err := q.QueryContext(ctx, `SELECT key FROM settings WHERE is_encrypted = TRUE`)
	if err != nil {
		return err
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		names = append(names, name)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, name := range names {
		value, _, err := Get(ctx, q, name, old)
		if err != nil {
			return err
		}
		if err := Set(ctx, q, name, value, true, master); err != nil {
			return err
		}
	}
	if err := InitializeMaster(ctx, q, master); err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `DELETE FROM app_auth`)
	return err
}
