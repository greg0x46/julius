package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/google/uuid"
	"github.com/greg0x46/julius/internal/httpapi"
	"github.com/greg0x46/julius/internal/marketdata"
	"github.com/greg0x46/julius/internal/quotes"
	"github.com/greg0x46/julius/internal/settings"
	"github.com/greg0x46/julius/internal/worker"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Operations receives the same services and shutdown context as the REST server.
type Operations struct {
	Context      context.Context
	Market       *marketdata.Service
	Secrets      *settings.Secrets
	SyncSchedule *worker.Schedule
}

type operationInput struct {
	IdempotencyKey string `json:"idempotency_key" jsonschema:"8-128 printable ASCII characters, no spaces or commas. Reuse the same key and arguments on retries for one hour, within this server process and access token."`
	SourceID       string `json:"source_id,omitempty" jsonschema:"Sync only: optional active Pluggy connection UUID; omitted means all active connections."`
}
type operationOutput struct {
	RequestID string                  `json:"request_id"`
	State     string                  `json:"state"`
	Result    *httpapi.MCPWriteResult `json:"result,omitempty"`
	Error     string                  `json:"error,omitempty"`
}
type operationReceipt struct {
	Owner, Operation string
	Input            operationInput
	Expires          time.Time
	Output           operationOutput
}

func operationTool[In, Out any](server *sdk.Server, name, description string, readOnly bool, call func(context.Context, In) (Out, error)) {
	schema, err := jsonschema.For[Out](&jsonschema.ForOptions{TypeSchemas: map[reflect.Type]*jsonschema.Schema{reflect.TypeFor[json.RawMessage](): {}}})
	if err != nil {
		panic(err)
	}
	sdk.AddTool(server, &sdk.Tool{Name: name, Description: description, OutputSchema: schema, Annotations: &sdk.ToolAnnotations{ReadOnlyHint: readOnly, IdempotentHint: true}}, func(ctx context.Context, _ *sdk.CallToolRequest, in In) (*sdk.CallToolResult, Out, error) {
		out, err := call(ctx, in)
		return nil, out, err
	})
}

func (o *oauthServer) registerOperations(server *sdk.Server, ops Operations) {
	var mu sync.Mutex
	receipts := map[string]*operationReceipt{}
	operationTool(server, "get_operations_status", "Read worker readiness, daily schedules and the last full quote refresh. No credentials or environment values. Manual requests do not change schedules; disabled automatic quotations still permit explicit manual refresh.", true, func(ctx context.Context, _ struct{}) (httpapi.MCPWriteResult, error) {
		config, _, err := settings.GetQuoteRefresh(ctx, o.store.DB)
		if err != nil {
			return httpapi.MCPOperationData(nil, err)
		}
		last, err := quotes.GetLastRefresh(ctx, o.store.DB)
		var schedule any
		if ops.SyncSchedule != nil {
			schedule = map[string]any{"time": time.Date(2000, 1, 1, ops.SyncSchedule.Hour, ops.SyncSchedule.Minute, 0, 0, time.UTC).Format("15:04"), "timezone": ops.SyncSchedule.Location.String()}
		}
		return httpapi.MCPOperationData(map[string]any{"sync_ready": worker.Ready(ctx, o.store.DB, ops.Secrets), "sync_schedule": schedule, "quotes_available": ops.Market != nil, "quotes_schedule": config, "last_quote_refresh": last}, err)
	})
	type historyInput struct {
		ID     string `json:"id,omitempty" jsonschema:"For get_sync_run: synchronization UUID; omitted for list_sync_runs."`
		Limit  int    `json:"limit,omitempty" jsonschema:"1-100; default 20. For a detail, bounds the failures page."`
		Offset int    `json:"offset,omitempty" jsonschema:"Zero-based offset; default 0."`
	}
	for _, name := range []string{"list_sync_runs", "get_sync_run"} {
		operationTool(server, name, "Read durable provider sync status and history. get_sync_run requires id and returns a page of canonical safe failures; list_sync_runs forbids id. Pages have at most 100 items; fewer than limit means the end. No raw provider responses.", true, func(ctx context.Context, in historyInput) (httpapi.MCPWriteResult, error) {
			if in.Limit < 0 || in.Limit > 100 || in.Offset < 0 || (name == "get_sync_run" && !validOperationID(in.ID)) || (name == "list_sync_runs" && in.ID != "") {
				return httpapi.MCPWriteResult{}, errors.New("invalid history arguments")
			}
			if in.Limit == 0 {
				in.Limit = 20
			}
			value, err := httpapi.MCPReadSync(ctx, o.store.DB, in.ID, in.Limit, in.Offset)
			return httpapi.MCPOperationData(value, err)
		})
	}
	for _, name := range []string{"request_sync", "request_quote_refresh"} {
		operationTool(server, name, "Request maintenance, requiring mcp:access mcp:write mcp:operations. OAuth consent authorizes this action without a separate financial approval. Required idempotency_key prevents duplicate retries for one hour in this process/token. Poll get_operation_result. Sync queues durable run IDs for get_sync_run; quotes use the shared scheduler/REST guard, provider cache and retry policy. source_id is sync-only. Never changes schedules or configuration.", false, func(ctx context.Context, in operationInput) (operationOutput, error) {
			identity, _ := ctx.Value(writeContextKey{}).(writeIdentity)
			if !canOperate(identity.Scope) {
				return operationOutput{}, errors.New("mcp:write and mcp:operations scopes required")
			}
			if !validOperationKey(in.IdempotencyKey) || (in.SourceID != "" && (!validOperationID(in.SourceID) || name != "request_sync")) {
				return operationOutput{}, errors.New("invalid operation arguments")
			}
			mu.Lock()
			defer mu.Unlock()
			now := time.Now()
			for id, r := range receipts {
				if r.Output.State != "running" && !now.Before(r.Expires) {
					delete(receipts, id)
					continue
				}
				if r.Owner == identity.Hash && r.Input.IdempotencyKey == in.IdempotencyKey {
					if r.Operation != name || r.Input != in {
						return operationOutput{}, errors.New("idempotency key already used with different arguments")
					}
					return r.Output, nil
				}
			}
			if len(receipts) >= 100 {
				return operationOutput{}, errors.New("too many operation receipts; wait for expiry")
			}
			if ops.Context == nil || ops.Context.Err() != nil {
				return operationOutput{}, errors.New("operations unavailable")
			}
			if name == "request_sync" && !worker.Ready(ctx, o.store.DB, ops.Secrets) {
				return operationOutput{}, errors.New("synchronization unavailable; configure Pluggy in Julius")
			}
			if name == "request_quote_refresh" {
				if ops.Market == nil {
					return operationOutput{}, errors.New("quotes unavailable")
				}
				for _, r := range receipts {
					if r.Operation == name && r.Output.State == "running" {
						return operationOutput{}, errors.New("quote refresh already running; wait before retrying")
					}
				}
			}
			id := uuid.NewString()
			receipt := &operationReceipt{Owner: identity.Hash, Operation: name, Input: in, Expires: now.Add(time.Hour), Output: operationOutput{RequestID: id, State: "running"}}
			receipts[id] = receipt
			if name == "request_sync" {
				result := httpapi.MCPRequestSync(ctx, o.store.DB, in.SourceID)
				receipt.Output.State = "accepted"
				if result.Status != 202 {
					receipt.Output.State = "rejected"
				}
				receipt.Output.Result = &result
			} else {
				go func() {
					// Disconnects do not cancel accepted work; shutdown and this deadline do.
					runCtx, cancel := context.WithTimeout(ops.Context, 5*time.Minute)
					defer cancel()
					last, err := quotes.RefreshNow(runCtx, o.store.DB, ops.Market)
					mu.Lock()
					defer mu.Unlock()
					receipt.Output.State = "finished"
					if err != nil {
						receipt.Output.State = "rejected"
						receipt.Output.Error = "quote refresh already running; retry with a new key after it finishes"
						return
					}
					result, resultErr := httpapi.MCPOperationData(last, nil)
					if resultErr != nil {
						receipt.Output.State = "failed"
						receipt.Output.Error = "quote refresh result unavailable"
						return
					}
					receipt.Output.Result = &result
				}()
			}
			return receipt.Output, nil
		})
	}
	operationTool(server, "get_operation_result", "Poll an operation receipt using the exact original access token and operations scopes. Process-local receipts expire after one hour; after restart inspect durable sync history/last_quote_refresh before deciding to request new work. accepted means queued, not synchronized; finished quotes may still report failed, partial or deferred. Quote history backfill runs separately.", true, func(ctx context.Context, in struct {
		RequestID string `json:"request_id"`
	}) (operationOutput, error) {
		identity, _ := ctx.Value(writeContextKey{}).(writeIdentity)
		if !canOperate(identity.Scope) {
			return operationOutput{}, errors.New("mcp:write and mcp:operations scopes required")
		}
		if !validOperationID(in.RequestID) {
			return operationOutput{}, errors.New("invalid request_id")
		}
		mu.Lock()
		defer mu.Unlock()
		receipt := receipts[in.RequestID]
		if receipt == nil || receipt.Owner != identity.Hash || (receipt.Output.State != "running" && !time.Now().Before(receipt.Expires)) {
			return operationOutput{}, errors.New("operation unavailable or expired")
		}
		return receipt.Output, nil
	})
}
func validOperationID(id string) bool { _, err := uuid.Parse(id); return err == nil }
func validOperationKey(key string) bool {
	if len(key) < 8 || len(key) > 128 {
		return false
	}
	for _, c := range key {
		if c < 33 || c > 126 || c == ',' {
			return false
		}
	}
	return true
}
