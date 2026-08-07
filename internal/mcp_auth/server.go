package mcp_auth

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/ory/fosite"
	"github.com/ory/fosite/compose"

	"recipes/internal/config"
	"recipes/internal/database"
	mongorepo "recipes/internal/database/mongo"
	"recipes/internal/utils"
)

var authorizePage = template.Must(template.New("authorize").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width">
<title>Connect {{.ClientName}}</title><style>
body{font:16px system-ui;max-width:34rem;margin:4rem auto;padding:0 1rem;color:#222}label{display:block;margin:.8rem 0 .25rem}
input{box-sizing:border-box;width:100%;padding:.7rem}.actions{display:flex;gap:.7rem;margin-top:1.2rem}button{padding:.7rem 1rem}
.error{color:#a00}.scopes{background:#f4f4f4;padding:1rem;border-radius:.4rem}
</style></head><body><h1>Connect {{.ClientName}}</h1>
<p>Sign in and allow this agent to use the selected recipe permissions.</p>
<div class="scopes">{{range .Scopes}}<div>• {{.}}</div>{{end}}</div>
{{if .Error}}<p class="error">{{.Error}}</p>{{end}}
<form method="post"><label for="identifier">Username or email</label><input id="identifier" name="identifier" autocomplete="username" required>
<label for="password">Password</label><input id="password" type="password" name="password" autocomplete="current-password" required>
<div class="actions"><button name="decision" value="allow" type="submit">Allow</button><button name="decision" value="deny" type="submit" formnovalidate>Deny</button></div></form>
</body></html>`))

type Server struct {
	provider    fosite.OAuth2Provider
	clients     *MetadataResolver
	db          database.Database
	secret      []byte
	issuer      string
	publicURL   string
	metadataURL string
}

func NewServer(ctx context.Context, cfg *config.MCPConfig, db database.Database) (*Server, error) {
	parsed, err := url.Parse(cfg.PublicURL)
	if err != nil {
		return nil, err
	}
	issuer := parsed.Scheme + "://" + parsed.Host
	secret := []byte(cfg.JWTSecret)
	if len(secret) == 0 {
		secret = make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return nil, err
		}
		log.Printf("warning: RECIPES_MCP_JWTSECRET is unset; MCP tokens will be invalid after restart")
	}
	clients := NewMetadataResolver(cfg.PublicURL)
	store, err := NewMongoStore(ctx, db.RawDatabase(), clients, cfg.AuthorizationCodeTTL)
	if err != nil {
		return nil, err
	}
	fositeConfig := &fosite.Config{
		AccessTokenLifespan: cfg.AccessExpiration, RefreshTokenLifespan: cfg.RefreshExpiration,
		AuthorizeCodeLifespan: cfg.AuthorizationCodeTTL, GlobalSecret: secret,
		EnforcePKCE: true, EnablePKCEPlainChallengeMethod: false, RefreshTokenScopes: []string{},
		ScopeStrategy: fosite.ExactScopeStrategy, AccessTokenIssuer: issuer,
		TokenURL: issuer + "/oauth/token", SendDebugMessagesToClients: false,
	}
	core := compose.NewOAuth2HMACStrategy(fositeConfig)
	strategy := NewJWTAccessStrategy(core, secret, issuer, cfg.PublicURL)
	provider := compose.Compose(fositeConfig, store, strategy,
		compose.OAuth2AuthorizeExplicitFactory,
		compose.OAuth2RefreshTokenGrantFactory,
		compose.OAuth2PKCEFactory,
	)
	return &Server{
		provider: provider, clients: clients, db: db, secret: secret,
		issuer: issuer, publicURL: cfg.PublicURL,
		metadataURL: issuer + "/.well-known/oauth-protected-resource/mcp",
	}, nil
}

func (s *Server) ResourceMetadataURL() string { return s.metadataURL }

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (s *Server) ProtectedResourceMetadata(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"resource": s.publicURL, "authorization_servers": []string{s.issuer},
		"scopes_supported":         []string{"recipes:read", "recipes:write"},
		"bearer_methods_supported": []string{"header"},
	})
}

func (s *Server) AuthorizationServerMetadata(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer": s.issuer, "authorization_endpoint": s.issuer + "/oauth/authorize",
		"token_endpoint":                        s.issuer + "/oauth/token",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"token_endpoint_auth_methods_supported": []string{"none"},
		"code_challenge_methods_supported":      []string{"S256"},
		"scopes_supported":                      []string{"recipes:read", "recipes:write"},
		"client_id_metadata_document_supported": true,
		"resource_indicators_supported":         true,
	})
}

func (s *Server) validateResource(r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return fosite.ErrInvalidRequest.WithWrap(err)
	}
	resources := r.Form["resource"]
	if r.Form.Get("grant_type") == "refresh_token" && len(resources) == 0 {
		// RFC 8707 carries the originally granted resource forward on refresh.
		// The official Go MCP client uses the standard oauth2 token source, which
		// omits this optional parameter on refresh requests.
		return nil
	}
	if len(resources) != 1 || resources[0] != s.publicURL {
		return fosite.ErrInvalidRequest.WithHint("resource must identify this MCP server")
	}
	return nil
}

func validScopes(scopes fosite.Arguments) bool {
	return len(scopes) > 0 && slices.ContainsFunc(scopes, func(scope string) bool {
		return scope != "recipes:read" && scope != "recipes:write"
	}) == false
}

func (s *Server) renderAuthorize(w http.ResponseWriter, r *http.Request, ar fosite.AuthorizeRequester, message string) {
	metadata, _, err := s.clients.Resolve(r.Context(), ar.GetClient().GetID())
	if err != nil {
		s.provider.WriteAuthorizeError(r.Context(), w, ar, fosite.ErrInvalidClient.WithWrap(err))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'")
	w.Header().Set("X-Frame-Options", "DENY")
	_ = authorizePage.Execute(w, map[string]any{"ClientName": metadata.ClientName, "Scopes": ar.GetRequestedScopes(), "Error": message})
}

func (s *Server) Authorize(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.validateResource(r); err != nil {
		s.provider.WriteAuthorizeError(ctx, w, nil, err)
		return
	}
	if r.Form.Get("scope") == "" {
		r.Form.Set("scope", "recipes:read recipes:write")
	}
	ar, err := s.provider.NewAuthorizeRequest(ctx, r)
	if err != nil {
		s.provider.WriteAuthorizeError(ctx, w, ar, err)
		return
	}
	if !validScopes(ar.GetRequestedScopes()) {
		s.provider.WriteAuthorizeError(ctx, w, ar, fosite.ErrInvalidScope)
		return
	}
	if r.Method == http.MethodGet {
		s.renderAuthorize(w, r, ar, "")
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.FormValue("decision") != "allow" {
		s.provider.WriteAuthorizeError(ctx, w, ar, fosite.ErrAccessDenied)
		return
	}
	identifier := strings.TrimSpace(r.FormValue("identifier"))
	user, loginErr := s.db.GetUserByIdentifier(identifier)
	if loginErr != nil && !errors.Is(loginErr, mongorepo.UserNotFoundError) {
		log.Printf("MCP login lookup failed: %v", loginErr)
	}
	if loginErr != nil || utils.ComparePasswords(user.Password, r.FormValue("password")) != nil {
		s.renderAuthorize(w, r, ar, "Invalid username, email, or password.")
		return
	}
	for _, scope := range ar.GetRequestedScopes() {
		ar.GrantScope(scope)
	}
	ar.GrantAudience(s.publicURL)
	session := &fosite.DefaultSession{Username: user.Username, Subject: user.Id.Hex(), Extra: map[string]any{"auth_version": user.MCPAuthVersion}}
	response, err := s.provider.NewAuthorizeResponse(ctx, ar, session)
	if err != nil {
		s.provider.WriteAuthorizeError(ctx, w, ar, err)
		return
	}
	response.AddParameter("iss", s.issuer)
	s.provider.WriteAuthorizeResponse(ctx, w, ar, response)
}

func sessionAuthVersion(session *fosite.DefaultSession) int64 { return authVersion(session) }

func (s *Server) Token(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	ctx := r.Context()
	if err := s.validateResource(r); err != nil {
		s.provider.WriteAccessError(ctx, w, nil, err)
		return
	}
	session := new(fosite.DefaultSession)
	ar, err := s.provider.NewAccessRequest(ctx, r, session)
	if err != nil {
		s.provider.WriteAccessError(ctx, w, ar, err)
		return
	}
	user, err := s.db.GetUserById(session.Subject)
	if err != nil || user.MCPAuthVersion != sessionAuthVersion(session) {
		s.provider.WriteAccessError(ctx, w, ar, fosite.ErrInvalidGrant)
		return
	}
	response, err := s.provider.NewAccessResponse(ctx, ar)
	if err != nil {
		s.provider.WriteAccessError(ctx, w, ar, err)
		return
	}
	s.provider.WriteAccessResponse(ctx, w, ar, response)
}

func (s *Server) TokenVerifier() auth.TokenVerifier {
	return func(ctx context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		claims, err := ParseAccessToken(token, s.secret, s.issuer, s.publicURL)
		if err != nil {
			return nil, fmt.Errorf("%w: access token rejected", auth.ErrInvalidToken)
		}
		user, err := s.db.GetUserById(claims.Subject)
		if err != nil || user.MCPAuthVersion != claims.AuthVersion {
			return nil, fmt.Errorf("%w: account credentials changed", auth.ErrInvalidToken)
		}
		if claims.ExpiresAt == nil {
			return nil, fmt.Errorf("%w: token has no expiration", auth.ErrInvalidToken)
		}
		return &auth.TokenInfo{
			Scopes: strings.Fields(claims.Scope), Expiration: claims.ExpiresAt.Time, UserID: claims.Subject,
			Extra: map[string]any{"client_id": claims.ClientID, "token_id": claims.ID},
		}, nil
	}
}
