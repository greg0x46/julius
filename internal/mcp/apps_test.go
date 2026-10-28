package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"io/fs"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/httpapi"
	"github.com/greg0x46/julius/internal/investments"
	"github.com/greg0x46/julius/internal/webui"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/shopspring/decimal"
)

func TestPortfolioAppSharedContract(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		today := investments.ProviderDay(time.Now())
		account, err := investments.CreateAccount(ctx, conn, investments.AccountInput{Name: "Synthetic custody"})
		if err != nil {
			t.Fatal(err)
		}
		_, err = investments.CreatePosition(ctx, conn, investments.PositionInput{AccountID: account.ID, Name: "Synthetic unpriced position", AssetType: "CDB", InitialQuantity: decimal.NewFromInt(1), InitialUnitCost: decimal.RequireFromString("123.456789012345678"), OccurredOn: today.AddDate(0, 0, -1)})
		if err != nil {
			t.Fatal(err)
		}
		var fallback map[string]any
		for _, mode := range []string{"no-asset", "ordinary", "apps"} {
			withUI := mode != "no-asset"
			server := sdk.NewServer(&sdk.Implementation{Name: "contract-test", Version: "1"}, nil)
			uri := ""
			options := &sdk.ClientOptions{}
			if withUI {
				uri = addPortfolioApp(server, "<!doctype html><html lang=\"pt-BR\"><body>Static resource</body></html>")
				if mode == "apps" {
					options.Capabilities = &sdk.ClientCapabilities{Extensions: map[string]any{"io.modelcontextprotocol/ui": map[string]any{"mimeTypes": []string{appMIME}}}}
				}
			}
			httpapi.RegisterMCPPlanningTools(server, conn, uri)
			st, ct := sdk.NewInMemoryTransports()
			ss, err := server.Connect(ctx, st, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer ss.Close()
			client := sdk.NewClient(&sdk.Implementation{Name: "portable-client", Version: "1"}, options)
			session, err := client.Connect(ctx, ct, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			list, err := session.ListTools(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, tool := range list.Tools {
				if tool.Name == "get_investment_portfolio" {
					if tool.OutputSchema == nil || tool.InputSchema == nil || !tool.Annotations.ReadOnlyHint {
						t.Fatal("missing read-only typed contract")
					}
					if withUI && tool.Meta["ui"].(map[string]any)["resourceUri"] != portfolioAppURI {
						t.Fatal("missing portable UI resource metadata")
					}
					if !withUI && tool.Meta["ui"] != nil {
						t.Fatal("fallback requires UI")
					}
				} else if tool.Meta["ui"] != nil {
					t.Fatal("UI leaked to another tool")
				}
			}
			result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "get_investment_portfolio", Arguments: map[string]any{"to": today.Format(time.DateOnly)}})
			if err != nil || result.IsError {
				t.Fatalf("portfolio: %v, %+v", err, result)
			}
			if len(result.Content) != 1 {
				t.Fatal("missing complete text fallback")
			}
			text, ok := result.Content[0].(*sdk.TextContent)
			if !ok {
				t.Fatal("fallback is not text")
			}
			var textual, structured map[string]any
			if err := json.Unmarshal([]byte(text.Text), &textual); err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(result.StructuredContent)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(raw, &structured); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(textual, structured) {
				t.Fatal("text loses portfolio data")
			}
			totals := structured["totals"].(map[string]any)
			if totals["partial"] != true || totals["positions_unavailable"] != float64(1) {
				t.Fatal("unavailable position was lost")
			}
			// Identity FX timestamps can differ, but every portfolio field is identical.
			delete(structured, "fx")
			if !withUI {
				fallback = structured
			} else {
				if !reflect.DeepEqual(fallback, structured) {
					t.Fatal("UI changes shared financial output")
				}
				resource, err := session.ReadResource(ctx, &sdk.ReadResourceParams{URI: portfolioAppURI})
				if err != nil {
					t.Fatal(err)
				}
				content := resource.Contents[0]
				if content.MIMEType != appMIME || strings.Contains(content.Text, "Synthetic") {
					t.Fatal("resource contains financial data or wrong MIME")
				}
				csp := content.Meta["ui"].(map[string]any)["csp"].(map[string]any)
				for _, key := range []string{"connectDomains", "resourceDomains", "frameDomains", "baseUriDomains"} {
					if len(csp[key].([]any)) != 0 {
						t.Fatalf("resource allows %s", key)
					}
				}
			}
		}
	})
}

func TestPortfolioAppAuthorization(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		store, token := setup(t, conn)
		handler, err := NewHandler(true, testConfig, store)
		if err != nil {
			t.Fatal(err)
		}
		body := `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"` + portfolioAppURI + `"}}`
		for _, method := range []string{body, `{"jsonrpc":"2.0","id":1,"method":"resources/list"}`} {
			for _, credential := range []string{"", "invalid"} {
				if got := request(t, handler, credential, "", method).Code; got != http.StatusUnauthorized {
					t.Fatalf("resource authorization: %d", got)
				}
			}
			if got := request(t, handler, token, "https://evil.example", method).Code; got != http.StatusForbidden {
				t.Fatalf("resource origin: %d", got)
			}
		}
		response := request(t, handler, token, "", body)
		if response.Code != 200 {
			t.Fatalf("read resource: %d", response.Code)
		}
		assets, err := webui.DistFS()
		if err != nil {
			t.Fatal(err)
		}
		_, err = fs.Stat(assets, "mcp-investments.html")
		if err == nil && !strings.Contains(response.Body.String(), appMIME) {
			t.Fatal("embedded resource unavailable")
		}
		if err != nil && !strings.Contains(response.Body.String(), `"error"`) {
			t.Fatal("unbuilt resource unexpectedly advertised")
		}
		if _, err := conn.Exec(`UPDATE oauth_grants SET scope='mcp:write' WHERE token_hash=?`, tokenHash(token)); err != nil {
			t.Fatal(err)
		}
		if got := request(t, handler, token, "", body).Code; got != http.StatusForbidden {
			t.Fatalf("missing access scope: %d", got)
		}
		if _, err := conn.Exec(`DELETE FROM oauth_grants WHERE token_hash=?`, tokenHash(token)); err != nil {
			t.Fatal(err)
		}
		if got := request(t, handler, token, "", body).Code; got != http.StatusUnauthorized {
			t.Fatalf("revoked resource access: %d", got)
		}
	})
}
