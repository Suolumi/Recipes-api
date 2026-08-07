package mcp_auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ory/fosite"
)

const maxClientMetadataBytes = 64 << 10

type ClientMetadata struct {
	ClientID                string   `json:"client_id"`
	ClientName              string   `json:"client_name"`
	ClientURI               string   `json:"client_uri,omitempty"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types,omitempty"`
	ResponseTypes           []string `json:"response_types,omitempty"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method,omitempty"`
}

type cachedClient struct {
	metadata *ClientMetadata
	client   *fosite.DefaultClient
	expires  time.Time
}

type MetadataResolver struct {
	publicURL  string
	allowLocal bool
	httpClient *http.Client
	mu         sync.Mutex
	cache      map[string]cachedClient
}

func NewMetadataResolver(publicURL string) *MetadataResolver {
	parsed, _ := url.Parse(publicURL)
	allowLocal := isLocalHostname(parsed.Hostname())
	resolver := &MetadataResolver{publicURL: publicURL, allowLocal: allowLocal, cache: map[string]cachedClient{}}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
			if err != nil {
				return nil, err
			}
			for _, ip := range ips {
				if allowLocal || isPublicIP(ip) {
					return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				}
			}
			return nil, errors.New("client metadata host resolves to a blocked address")
		},
	}
	resolver.httpClient = &http.Client{
		Transport: transport,
		Timeout:   7 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("too many client metadata redirects")
			}
			return resolver.validateMetadataURL(req.URL)
		},
	}
	return resolver
}

func isLocalHostname(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func isPublicIP(ip netip.Addr) bool {
	return ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast()
}

func (r *MetadataResolver) validateMetadataURL(parsed *url.URL) error {
	if parsed == nil || parsed.Host == "" || parsed.Path == "" || parsed.Path == "/" || parsed.Fragment != "" {
		return errors.New("client_id must be an absolute metadata URL with a path")
	}
	if parsed.Scheme != "https" && !(r.allowLocal && parsed.Scheme == "http" && isLocalHostname(parsed.Hostname())) {
		return errors.New("client metadata must use HTTPS")
	}
	if parsed.User != nil {
		return errors.New("client metadata URL cannot contain user information")
	}
	return nil
}

func validateRedirectURI(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.Fragment != "" {
		return errors.New("invalid redirect URI")
	}
	if parsed.Scheme == "https" {
		return nil
	}
	if parsed.Scheme == "http" && isLocalHostname(parsed.Hostname()) {
		return nil
	}
	return errors.New("redirect URIs must use HTTPS, except loopback development callbacks")
}

func (r *MetadataResolver) Resolve(ctx context.Context, id string) (*ClientMetadata, *fosite.DefaultClient, error) {
	r.mu.Lock()
	if cached, ok := r.cache[id]; ok && cached.expires.After(time.Now()) {
		r.mu.Unlock()
		return cached.metadata, cached.client, nil
	}
	r.mu.Unlock()

	parsed, err := url.Parse(id)
	if err != nil {
		return nil, nil, err
	}
	if err := r.validateMetadataURL(parsed); err != nil {
		return nil, nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, id, nil)
	if err != nil {
		return nil, nil, err
	}
	request.Header.Set("Accept", "application/json")
	response, err := r.httpClient.Do(request)
	if err != nil {
		return nil, nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("client metadata returned HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxClientMetadataBytes+1))
	if err != nil {
		return nil, nil, err
	}
	if len(data) > maxClientMetadataBytes {
		return nil, nil, errors.New("client metadata document is too large")
	}
	var metadata ClientMetadata
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	if err := decoder.Decode(&metadata); err != nil {
		return nil, nil, fmt.Errorf("invalid client metadata: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, nil, errors.New("invalid client metadata: expected one JSON object")
	}
	if metadata.ClientID != id || strings.TrimSpace(metadata.ClientName) == "" || len(metadata.RedirectURIs) == 0 {
		return nil, nil, errors.New("client metadata is missing required fields or has a mismatched client_id")
	}
	if metadata.TokenEndpointAuthMethod != "none" {
		return nil, nil, errors.New("only public clients using token_endpoint_auth_method=none are supported")
	}
	if len(metadata.GrantTypes) == 0 {
		metadata.GrantTypes = []string{"authorization_code"}
	}
	if !slices.Contains(metadata.GrantTypes, "authorization_code") {
		return nil, nil, errors.New("client does not support the authorization_code grant")
	}
	if len(metadata.ResponseTypes) == 0 {
		metadata.ResponseTypes = []string{"code"}
	}
	if !slices.Contains(metadata.ResponseTypes, "code") {
		return nil, nil, errors.New("client does not support the code response type")
	}
	for _, redirectURI := range metadata.RedirectURIs {
		if err := validateRedirectURI(redirectURI); err != nil {
			return nil, nil, err
		}
	}
	client := &fosite.DefaultClient{
		ID: id, RedirectURIs: metadata.RedirectURIs, GrantTypes: metadata.GrantTypes,
		ResponseTypes: metadata.ResponseTypes, Scopes: []string{"recipes:read", "recipes:write"},
		Audience: []string{r.publicURL}, Public: true,
	}
	r.mu.Lock()
	r.cache[id] = cachedClient{metadata: &metadata, client: client, expires: time.Now().Add(10 * time.Minute)}
	r.mu.Unlock()
	return &metadata, client, nil
}
