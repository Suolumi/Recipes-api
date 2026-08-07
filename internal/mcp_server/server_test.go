package mcp_server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestCursorRoundTrip(t *testing.T) {
	id := primitive.NewObjectID().Hex()
	encoded := encodeCursor(id)
	if encoded == "" || encoded == id {
		t.Fatalf("cursor was not made opaque: %q", encoded)
	}
	decoded, err := decodeCursor(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded != id {
		t.Fatalf("decoded cursor = %q, want %q", decoded, id)
	}
	if _, err := decodeCursor("not-a-cursor"); err == nil {
		t.Fatal("expected malformed cursor to fail")
	}
}

func TestUserWithScope(t *testing.T) {
	req := &mcp.CallToolRequest{Extra: &mcp.RequestExtra{TokenInfo: &auth.TokenInfo{UserID: "user-id", Scopes: []string{"recipes:read"}}}}
	userID, err := userWithScope(req, "recipes:read")
	if err != nil || userID != "user-id" {
		t.Fatalf("userWithScope() = %q, %v", userID, err)
	}
	if _, err := userWithScope(req, "recipes:write"); err == nil {
		t.Fatal("expected missing scope to fail")
	}
}

func TestProtocolOnly(t *testing.T) {
	handler := protocolOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))

	request := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	request.Header.Set("Mcp-Protocol-Version", protocolVersion)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("current protocol status = %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodPost, "/mcp", nil)
	request.Header.Set("Mcp-Protocol-Version", "2025-11-25")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("legacy protocol status = %d", response.Code)
	}
}
