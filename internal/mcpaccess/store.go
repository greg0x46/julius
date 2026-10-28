// Package mcpaccess owns persisted client authorizations, separate from browser sessions.
package mcpaccess

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/greg0x46/julius/internal/db"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/greg0x46/julius/internal/settings"
)

const Read = "mcp:read"
const ReadWrite = "mcp:read mcp:write"
const EnabledKey = "mcp.enabled"

var ErrScope = errors.New("invalid MCP scope")

func ValidScope(scope string) bool { return scope == Read || scope == ReadWrite }
func Enabled(ctx context.Context, q settings.Querier) (bool, error) {
	value, found, err := settings.Get(ctx, q, EnabledKey, nil)
	if err != nil {
		return false, err
	}
	if !found {
		return true, nil
	}
	if value != "true" && value != "false" {
		return false, errors.New("invalid MCP configuration")
	}
	return value == "true", nil
}
func SetEnabled(ctx context.Context, q settings.Querier, enabled bool) error {
	value := "false"
	if enabled {
		value = "true"
	}
	return settings.Set(ctx, q, EnabledKey, value, false, nil)
}

type Authorization struct {
	ID            string     `json:"id"`
	ClientID      string     `json:"client_id"`
	Name          string     `json:"client_name"`
	Scope         string     `json:"scope"`
	ApprovedScope string     `json:"approved_scope"`
	CreatedAt     time.Time  `json:"created_at"`
	LastUsedAt    *time.Time `json:"last_used_at"`
	ExpiresAt     time.Time  `json:"expires_at"`
	RevokedAt     *time.Time `json:"revoked_at"`
	Expired       bool       `json:"expired"`
}

type Store struct {
	DB  *sql.DB
	Now func() time.Time
}

func NewStore(conn *sql.DB) *Store { return &Store{DB: conn, Now: time.Now} }
func (s *Store) Create(ctx context.Context, client, scope string, revision int, expires time.Time) (string, error) {
	if !ValidScope(scope) {
		return "", ErrScope
	}
	id := uuid.NewString()
	result, err := s.DB.ExecContext(ctx, `INSERT INTO mcp_authorizations(id,client_id,scope,approved_scope,revision,created_at,expires_at) SELECT ?,?,?,?,?,?,? WHERE EXISTS(SELECT 1 FROM auth_owner WHERE id=1 AND revision=?)`, id, client, scope, scope, revision, db.FormatTime(s.Now()), db.FormatTime(expires), revision)
	if err != nil {
		return "", err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return "", err
	}
	if n != 1 {
		return "", sql.ErrNoRows
	}
	return id, nil
}
func (s *Store) List(ctx context.Context) ([]Authorization, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT a.id,a.client_id,c.name,a.scope,a.approved_scope,a.created_at,a.last_used_at,a.expires_at,a.revoked_at FROM mcp_authorizations a JOIN oauth_clients c ON c.id=a.client_id ORDER BY a.created_at DESC,a.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Authorization{}
	for rows.Next() {
		var a Authorization
		if err := rows.Scan(dbtypes.ID(&a.ID), &a.ClientID, &a.Name, &a.Scope, &a.ApprovedScope, dbtypes.Time(&a.CreatedAt), dbtypes.Time(&a.LastUsedAt), dbtypes.Time(&a.ExpiresAt), dbtypes.Time(&a.RevokedAt)); err != nil {
			return nil, err
		}
		a.Expired = !a.ExpiresAt.After(s.Now())
		result = append(result, a)
	}
	return result, rows.Err()
}
func (s *Store) SetScope(ctx context.Context, id, scope string) error {
	if !ValidScope(scope) {
		return ErrScope
	}
	result, err := s.DB.ExecContext(ctx, `UPDATE mcp_authorizations SET scope=? WHERE id=? AND revoked_at IS NULL AND expires_at>? AND revision=(SELECT revision FROM auth_owner WHERE id=1) AND (approved_scope=? OR ?=?)`, scope, id, db.FormatTime(s.Now()), scope, scope, Read)
	return changed(result, err)
}
func (s *Store) Revoke(ctx context.Context, id string) error {
	result, err := s.DB.ExecContext(ctx, `UPDATE mcp_authorizations SET revoked_at=COALESCE(revoked_at,?) WHERE id=?`, db.FormatTime(s.Now()), id)
	return changed(result, err)
}
func changed(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return sql.ErrNoRows
	}
	return nil
}
