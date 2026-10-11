package httpapi

import (
	"database/sql"
	"github.com/greg0x46/julius/internal/datasources"
	"github.com/greg0x46/julius/internal/quotes"
	"github.com/greg0x46/julius/internal/settings"
	"net/http"
)

func handleGetQuotesSettings(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		config, _, err := settings.GetQuoteRefresh(r.Context(), db)
		if err != nil {
			writeProblem(w, 503, "settings-unavailable", "Configuração indisponível", "")
			return
		}
		writeJSON(w, 200, config)
	}
}

func handleQuotesSettings(db *sql.DB, keys *settings.Secrets) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 16384)
		var req struct {
			BrapiToken *string `json:"brapi_token"`
			Enabled    *bool   `json:"enabled"`
			Time       *string `json:"time"`
			Timezone   *string `json:"timezone"`
		}
		if decodeStrict(r, &req) != nil {
			writeProblem(w, 422, "invalid-settings", "Dados inválidos", "")
			return
		}
		tx, err := db.BeginTx(r.Context(), nil)
		if err != nil {
			writeProblem(w, 503, "settings-unavailable", "Configuração indisponível", "")
			return
		}
		defer tx.Rollback()
		changed := req.Enabled != nil || req.Time != nil || req.Timezone != nil
		if changed {
			_, err := settings.InitializeQuoteRefresh(r.Context(), tx, settings.DefaultQuoteRefreshSettings())
			if err == nil {
				_, err = tx.ExecContext(r.Context(), `UPDATE settings SET value = value WHERE key = ?`, settings.KeyQuoteRefresh)
			}
			if err != nil {
				writeProblem(w, 503, "settings-unavailable", "Configuração indisponível", "")
				return
			}
			config, _, err := settings.GetQuoteRefresh(r.Context(), tx)
			if err != nil {
				writeProblem(w, 503, "settings-unavailable", "Configuração indisponível", "")
				return
			}
			if req.Enabled != nil {
				config.Enabled = *req.Enabled
			}
			if req.Time != nil {
				config.Time = *req.Time
			}
			if req.Timezone != nil {
				config.Timezone = *req.Timezone
			}
			if err := config.Validate(); err != nil {
				writeProblem(w, 422, "invalid-settings", "Dados inválidos", err.Error())
				return
			}
			if err := settings.SetQuoteRefresh(r.Context(), tx, config); err != nil {
				writeProblem(w, 503, "settings-unavailable", "Não foi possível salvar", "")
				return
			}
		}
		if req.BrapiToken != nil {
			key, ok := keys.Key()
			if !ok {
				writeProblem(w, 503, "settings-unavailable", "Configuração indisponível", "")
				return
			}
			if err := settings.SetBrapiToken(r.Context(), tx, *req.BrapiToken, key); err != nil {
				writeProblem(w, 503, "settings-unavailable", "Não foi possível salvar", "")
				return
			}
		}
		if err := tx.Commit(); err != nil {
			writeProblem(w, 503, "settings-unavailable", "Não foi possível salvar", "")
			return
		}
		if changed {
			quotes.ConfigurationChanged()
		}
		writeJSON(w, 200, map[string]bool{"saved": true})
	}
}

// Credentials are write-only and may be configured only by an authenticated owner.
func handlePluggySettings(db *sql.DB, keys *settings.Secrets) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 16384)
		var req struct {
			ClientID     string `json:"pluggy_client_id"`
			ClientSecret string `json:"pluggy_client_secret"`
			ItemID       string `json:"pluggy_item_id"`
		}
		if decodeStrict(r, &req) != nil || req.ClientID == "" || req.ClientSecret == "" {
			writeProblem(w, 422, "invalid-settings", "Credenciais inválidas", "Informe client ID e client secret da Pluggy.")
			return
		}
		key, ok := keys.Key()
		if !ok {
			writeProblem(w, 503, "settings-unavailable", "Configuração indisponível", "")
			return
		}
		tx, err := db.BeginTx(r.Context(), nil)
		if err == nil {
			defer tx.Rollback()
			err = settings.Set(r.Context(), tx, "pluggy.client_id", req.ClientID, true, key)
			if err == nil {
				err = settings.Set(r.Context(), tx, "pluggy.client_secret", req.ClientSecret, true, key)
			}
			if err == nil && req.ItemID != "" {
				_, err = datasources.Create(r.Context(), tx, datasources.ProviderPluggy, req.ItemID, nil)
			}
			if err == nil {
				err = tx.Commit()
			}
		}
		if err != nil {
			writeProblem(w, 503, "settings-unavailable", "Não foi possível salvar as credenciais", "")
			return
		}
		writeJSON(w, 200, map[string]bool{"saved": true})
	}
}
