package mcp

import (
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"time"

	"github.com/greg0x46/julius/internal/auth"
	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/mcpaccess"
)

func (o *oauthServer) csrfCookie() string {
	if o.config.Secure() {
		return "__Host-julius_oauth"
	}
	return "julius_oauth"
}
func (o *oauthServer) authorize(w http.ResponseWriter, r *http.Request) {
	if len(r.URL.RawQuery) > 8192 {
		oauthError(w, 400, "invalid_request")
		return
	}
	p, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		oauthError(w, 400, "invalid_request")
		return
	}
	for _, values := range p {
		if len(values) != 1 {
			oauthError(w, 400, "invalid_request")
			return
		}
	}
	g := grant{Client: p.Get("client_id"), Redirect: p.Get("redirect_uri"), Resource: p.Get("resource"), Scope: p.Get("scope"), Challenge: p.Get("code_challenge"), State: p.Get("state")}
	name, err := o.client(r, g.Client, g.Redirect)
	if errors.Is(err, sql.ErrNoRows) {
		oauthError(w, 400, "invalid_client")
		return
	}
	if err != nil {
		o.unavailable(w)
		return
	}
	if p.Get("response_type") != "code" {
		o.authorizationError(w, r, g, "unsupported_response_type")
		return
	}
	if g.Resource != o.resource() {
		o.authorizationError(w, r, g, "invalid_target")
		return
	}
	if g.Scope == "" {
		g.Scope = mcpScope
	}
	g.Scope = normalizeScope(g.Scope)
	if !validScope(g.Scope) {
		o.authorizationError(w, r, g, "invalid_scope")
		return
	}
	challenge, err := base64.RawURLEncoding.DecodeString(g.Challenge)
	if p.Get("code_challenge_method") != "S256" || err != nil || len(challenge) != 32 {
		o.authorizationError(w, r, g, "invalid_request")
		return
	}
	if _, err = o.store.DB.ExecContext(r.Context(), `DELETE FROM oauth_grants WHERE expires_at<=?`, db.FormatTime(o.store.Now())); err != nil {
		o.unavailable(w)
		return
	}
	csrf := randomToken()
	g.CSRF = tokenHash(csrf)
	pending := randomToken()
	if err = o.save(r, "consent", pending, g, codeLifetime); err != nil {
		o.unavailable(w)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: o.csrfCookie(), Value: csrf, Path: "/", Secure: o.config.Secure(), HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 300})
	o.renderConsent(w, 200, g, name, pending, "", "", mcpaccess.Read)
}
func (o *oauthServer) authorizationError(w http.ResponseWriter, r *http.Request, g grant, code string) {
	target, _ := url.Parse(g.Redirect)
	p := target.Query()
	p.Set("error", code)
	p.Set("state", g.State)
	p.Set("iss", o.config.PublicURL)
	target.RawQuery = p.Encode()
	http.Redirect(w, r, target.String(), http.StatusSeeOther)
}
func (o *oauthServer) consent(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Origin") != o.config.PublicURL {
		oauthError(w, 403, "access_denied")
		return
	}
	if !parseForm(w, r) {
		return
	}
	pending := r.PostForm.Get("request")
	g, err := o.load(r, "consent", pending, false)
	if errors.Is(err, sql.ErrNoRows) {
		oauthError(w, 400, "invalid_request")
		return
	}
	if err != nil {
		o.unavailable(w)
		return
	}
	cookie, err := r.Cookie(o.csrfCookie())
	if err != nil || subtle.ConstantTimeCompare([]byte(tokenHash(cookie.Value)), []byte(g.CSRF)) != 1 {
		oauthError(w, 403, "access_denied")
		return
	}
	decision := r.PostForm.Get("decision")
	if decision != "allow" && decision != "deny" {
		oauthError(w, 400, "invalid_request")
		return
	}
	selected := ""
	if decision == "allow" {
		// The owner may grant less than the client requested, never more.
		selected = r.PostForm.Get("scope")
		if selected == "" {
			selected = mcpaccess.Read
		}
		if !mcpaccess.ValidScope(selected) || mcpaccess.Rank(selected) > mcpaccess.Rank(g.Scope) {
			oauthError(w, 400, "invalid_scope")
			return
		}
		if o.limited(true) {
			w.Header().Set("Retry-After", "60")
			oauthError(w, 429, "temporarily_unavailable")
			return
		}
		select {
		case o.hashing <- struct{}{}:
			defer func() { <-o.hashing }()
		default:
			oauthError(w, 429, "temporarily_unavailable")
			return
		}
		revision, err := o.store.VerifyOwner(r.Context(), r.PostForm.Get("email"), r.PostForm.Get("password"))
		if errors.Is(err, auth.ErrCredentials) {
			name, err := o.client(r, g.Client, g.Redirect)
			if err != nil {
				o.unavailable(w)
				return
			}
			o.renderConsent(w, 401, g, name, pending, r.PostForm.Get("email"), "E-mail ou senha inválidos.", selected)
			return
		}
		if err != nil {
			o.unavailable(w)
			return
		}
		g.Revision = revision
		g.Scope = selected
	}
	if _, err = o.load(r, "consent", pending, true); errors.Is(err, sql.ErrNoRows) {
		oauthError(w, 400, "invalid_request")
		return
	} else if err != nil {
		o.unavailable(w)
		return
	}
	target, _ := url.Parse(g.Redirect)
	p := target.Query()
	p.Set("state", g.State)
	p.Set("iss", o.config.PublicURL)
	if decision == "deny" {
		p.Set("error", "access_denied")
	} else {
		access := mcpaccess.NewStore(o.store.DB)
		access.Now = o.store.Now
		id, err := access.Create(r.Context(), g.Client, g.Scope, g.Revision, o.store.Now().Add(accessLifetime+codeLifetime))
		if err != nil {
			o.unavailable(w)
			return
		}
		g.Authorization = &id
		code := randomToken()
		if err = o.save(r, "code", code, g, codeLifetime); err != nil {
			o.unavailable(w)
			return
		}
		p.Set("code", code)
	}
	target.RawQuery = p.Encode()
	http.SetCookie(w, &http.Cookie{Name: o.csrfCookie(), Value: "", Path: "/", Secure: o.config.Secure(), HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: -1, Expires: time.Unix(1, 0)})
	// A fresh document navigates independently of the consent form's same-origin CSP.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = callbackPage.Execute(w, target.String())
}
func (o *oauthServer) renderConsent(w http.ResponseWriter, status int, g grant, name, pending, email, message, selected string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = consentPage.Execute(w, struct {
		Grant                       grant
		CanWrite, CanOperate        bool
		Name, Request, Email, Error string
		Selected                    string
	}{g, canWrite(g.Scope), canOperate(g.Scope), name, pending, email, message, selected})
}

var consentPage = template.Must(template.New("consent").Parse(`<!doctype html>
<html lang="pt-BR"><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Autorizar acesso MCP · Julius</title>
<style>body{margin:0;background:#f5f7fa;color:#172b4d;font:16px system-ui,sans-serif}main{max-width:540px;margin:40px auto;padding:24px}h1{font-size:1.6rem}dt,label{font-weight:600}dd{margin:4px 0 16px;overflow-wrap:anywhere}label{display:block;margin:16px 0 6px}input,button{box-sizing:border-box;font:inherit;border-radius:8px;min-height:44px;padding:10px 14px}input{width:100%;border:1px solid #64748b}input[type=radio]{width:auto;min-height:auto}fieldset{margin-top:16px}button{cursor:pointer;margin:16px 8px 0 0;border:1px solid #2457a6;color:#2457a6;background:white}button[value=allow]{background:#2457a6;color:white}:focus-visible{outline:3px solid #2457a6;outline-offset:3px}[role=alert]{color:#c4302b}@media(max-width:480px){main{margin:0;padding:24px 16px}button{width:100%;margin-right:0}}</style>
<main><p>Julius</p><h1>Autorizar acesso MCP</h1><p>Este aplicativo solicita acesso ao seu Julius. Confira os dados antes de autorizar.</p>
<dl><dt>Aplicativo (nome informado pelo cliente)</dt><dd>{{if .Name}}{{.Name}}{{else}}Aplicativo sem nome{{end}}</dd><dt>Identificador do cliente</dt><dd>{{.Grant.Client}}</dd><dt>Endereço de retorno</dt><dd>{{.Grant.Redirect}}</dd><dt>Recurso</dt><dd>{{.Grant.Resource}}</dd><dt>Permissões</dt><dd>Consultar seus dados financeiros e o andamento das sincronizações via MCP (<code>{{.Grant.Scope}}</code>), incluindo investimentos, patrimônio, metas e projeções. {{if .CanWrite}}Solicitar alterações em transações manuais, categorias, inclusão nos totais, regras de categorização, metas, cenários e compromissos recorrentes, além de contas, posições e operações de investimento editáveis. Cada alteração financeira exige sua aprovação no Julius.{{else}}Acesso somente para leitura.{{end}} {{if .CanOperate}}Solicitar sincronizações e atualizações de cotações, conforme a configuração do Julius. Ao autorizar este acesso, você permite essas solicitações sem uma nova aprovação por operação.{{end}} Cenários hipotéticos entram nos cálculos apenas quando selecionados.</dd></dl>
<p>Confirme sua identidade para autorizar.</p>{{if .Error}}<p role="alert">{{.Error}}</p>{{end}}
<form action="/oauth/authorize" method="post"><input type="hidden" name="request" value="{{.Request}}"><fieldset><legend>Permissões deste cliente</legend><label><input type="radio" name="scope" value="mcp:access" {{if and (ne .Selected "mcp:access mcp:write") (ne .Selected "mcp:access mcp:write mcp:operations")}}checked{{end}}> Somente leitura</label>{{if .CanWrite}}<label><input type="radio" name="scope" value="mcp:access mcp:write" {{if eq .Selected "mcp:access mcp:write"}}checked{{end}}> Leitura e escrita</label>{{end}}{{if .CanOperate}}<label><input type="radio" name="scope" value="mcp:access mcp:write mcp:operations" {{if eq .Selected "mcp:access mcp:write mcp:operations"}}checked{{end}}> Leitura, escrita e operações</label>{{end}}</fieldset><p>Você pode reduzir as permissões ou revogar o acesso em Configurações → Segurança. Para ampliar além do autorizado aqui, será necessária uma nova autorização.</p><label for="email">E-mail</label><input id="email" name="email" type="email" autocomplete="username" value="{{.Email}}" required maxlength="254"><label for="password">Senha</label><input id="password" name="password" type="password" autocomplete="current-password" required><button type="submit" name="decision" value="allow">Autorizar acesso</button><button type="submit" name="decision" value="deny" formnovalidate>Negar acesso</button></form></main></html>`))

var callbackPage = template.Must(template.New("callback").Parse(`<!doctype html>
<html lang="pt-BR"><meta charset="utf-8"><meta http-equiv="refresh" content="0;url={{.}}"><title>Retornar ao aplicativo · Julius</title>
<p><a href="{{.}}">Continuar para o aplicativo</a></p></html>`))
