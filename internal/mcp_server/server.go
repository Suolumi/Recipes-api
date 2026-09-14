package mcp_server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"recipes/internal/config"
	"recipes/internal/models"
	"recipes/internal/recipe_service"
)

const protocolVersion = "2026-07-28"

type ListInput struct {
	Cursor string `json:"cursor,omitempty" jsonschema:"Opaque cursor returned by a previous call"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Page size from 1 to 100; defaults to 20"`
	Locale string `json:"locale,omitempty" jsonschema:"Requested BCP 47 locale; canonical recipe is returned if translation fails"`
}

type GetInput struct {
	RecipeID string `json:"recipe_id" jsonschema:"Recipe identifier"`
	Locale   string `json:"locale,omitempty" jsonschema:"Requested BCP 47 locale; canonical recipe is returned if translation fails"`
}

type CreateInput struct {
	Title           string              `json:"title"`
	Description     string              `json:"description,omitempty"`
	Quantity        int                 `json:"quantity"`
	Kind            models.RecipeKind   `json:"kind" jsonschema:"One of snack, starter, dish, side-dish, sauce, dessert, drink, or plate"`
	PreparationTime int                 `json:"preparation_time"`
	CookingTime     int                 `json:"cooking_time"`
	RestingTime     int                 `json:"resting_time"`
	Ingredients     []models.Ingredient `json:"ingredients"`
	Steps           []models.Step       `json:"steps"`
	Locale          string              `json:"locale,omitempty" jsonschema:"BCP 47 locale of the canonical recipe; detected when omitted"`
}

type UpdateInput struct {
	RecipeID        string               `json:"recipe_id"`
	Title           *string              `json:"title,omitempty"`
	Description     *string              `json:"description,omitempty"`
	Quantity        *int                 `json:"quantity,omitempty"`
	Kind            *models.RecipeKind   `json:"kind,omitempty"`
	PreparationTime *int                 `json:"preparation_time,omitempty"`
	CookingTime     *int                 `json:"cooking_time,omitempty"`
	RestingTime     *int                 `json:"resting_time,omitempty"`
	Ingredients     *[]models.Ingredient `json:"ingredients,omitempty"`
	Steps           *[]models.Step       `json:"steps,omitempty"`
	Locale          *string              `json:"locale,omitempty" jsonschema:"BCP 47 locale of the canonical recipe"`
	KeepPictureIDs  *[]string            `json:"keep_picture_ids,omitempty" jsonschema:"Ordered existing picture IDs to retain; omit to keep all, or pass an empty list to remove all"`
}

type PictureOutput struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// StepOutput mirrors models.Step but resolves Picture to a full URL (like
// PictureOutput does for recipe-level pictures) instead of a bare filename,
// which is meaningless to a caller without the server's picture base URL.
type StepOutput struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description"`
	Picture     string `json:"picture,omitempty"`
}

type RecipeOutput struct {
	ID              string              `json:"id"`
	Title           string              `json:"title"`
	Description     string              `json:"description"`
	Quantity        int                 `json:"quantity"`
	Kind            models.RecipeKind   `json:"kind"`
	PreparationTime int                 `json:"preparation_time"`
	CookingTime     int                 `json:"cooking_time"`
	RestingTime     int                 `json:"resting_time"`
	Ingredients     []models.Ingredient `json:"ingredients"`
	Steps           []StepOutput        `json:"steps"`
	Pictures        []PictureOutput     `json:"pictures"`
	SourceLocale    string              `json:"source_locale,omitempty"`
	Locale          string              `json:"locale,omitempty"`
}

type ListOutput struct {
	Items      []RecipeOutput `json:"items"`
	Total      int64          `json:"total"`
	NextCursor string         `json:"next_cursor,omitempty"`
}

type Server struct {
	handler     http.Handler
	recipes     *recipe_service.Service
	pictureBase string
}

func New(cfg *config.MCPConfig, recipes *recipe_service.Service, verifier auth.TokenVerifier) (*Server, error) {
	parsed, err := url.Parse(cfg.PublicURL)
	if err != nil {
		return nil, err
	}
	result := &Server{recipes: recipes, pictureBase: parsed.Scheme + "://" + parsed.Host + "/api/v1/recipe-pictures/"}
	server := mcp.NewServer(&mcp.Implementation{Name: "recipes", Version: "1.0.0"}, &mcp.ServerOptions{
		Instructions: "Manage only the authenticated user's recipes. Pictures cannot be uploaded through this MCP server; attach photos via the website.",
		Capabilities: &mcp.ServerCapabilities{},
	})
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method == "initialize" {
				return nil, fmt.Errorf("legacy MCP initialization is unsupported; use protocol %s", protocolVersion)
			}
			result, err := next(ctx, method, req)
			if err == nil && method == "server/discover" {
				if discovery, ok := result.(*mcp.DiscoverResult); ok {
					discovery.SupportedVersions = []string{protocolVersion}
				}
			}
			return result, err
		}
	})
	mcp.AddTool(server, &mcp.Tool{Name: "list_my_recipes", Description: "List the authenticated user's recipes with cursor pagination and optional localization."}, result.list)
	mcp.AddTool(server, &mcp.Tool{Name: "get_my_recipe", Description: "Get one recipe owned by the authenticated user, optionally localized."}, result.get)
	mcp.AddTool(server, &mcp.Tool{Name: "create_recipe", Description: "Create a complete recipe owned by the authenticated user. Pictures are not supported here; attach them via the website."}, result.create)
	mcp.AddTool(server, &mcp.Tool{Name: "update_recipe", Description: "Patch a recipe owned by the authenticated user and optionally reorder or remove existing pictures via keep_picture_ids. New pictures cannot be uploaded here; attach them via the website."}, result.update)
	stream := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{
		Stateless: true, JSONResponse: true, MaxRequestBodyBytes: 90 << 20, PropagateRequestCancellation: true,
	})
	protected := auth.RequireBearerToken(verifier, &auth.RequireBearerTokenOptions{})(stream)
	result.handler = cors(protocolOnly(protected))
	return result, nil
}

func (s *Server) Handler() http.Handler { return s.handler }

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Mcp-Protocol-Version")
		w.Header().Set("Access-Control-Expose-Headers", "WWW-Authenticate, Mcp-Protocol-Version")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func protocolOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if version := r.Header.Get("Mcp-Protocol-Version"); version != "" && version != protocolVersion {
			http.Error(w, "unsupported MCP protocol version", http.StatusBadRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func userWithScope(req *mcp.CallToolRequest, scope string) (string, error) {
	var info *auth.TokenInfo
	if req != nil && req.Extra != nil {
		info = req.Extra.TokenInfo
	}
	if info == nil || info.UserID == "" {
		return "", errors.New("authentication context is missing")
	}
	if !slices.Contains(info.Scopes, scope) {
		return "", fmt.Errorf("the token does not grant %s", scope)
	}
	return info.UserID, nil
}

func decodeCursor(cursor string) (string, error) {
	if cursor == "" {
		return "", nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil || len(decoded) != 12 {
		return "", errors.New("invalid cursor")
	}
	return primitive.ObjectID(decoded).Hex(), nil
}

func encodeCursor(cursor string) string {
	id, err := primitive.ObjectIDFromHex(cursor)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(id[:])
}

func (s *Server) stepOutputs(steps []models.Step) []StepOutput {
	outputs := make([]StepOutput, 0, len(steps))
	for _, step := range steps {
		output := StepOutput{Title: step.Title, Description: step.Description}
		if step.Picture != "" {
			output.Picture = s.pictureBase + step.Picture
		}
		outputs = append(outputs, output)
	}
	return outputs
}

func (s *Server) recipeOutput(recipe models.Recipe) RecipeOutput {
	output := RecipeOutput{
		Title: recipe.Title, Description: recipe.Description, Quantity: recipe.Quantity, Kind: recipe.Kind,
		PreparationTime: recipe.PreparationTime, CookingTime: recipe.CookingTime, RestingTime: recipe.RestingTime,
		Ingredients: recipe.Ingredients, Steps: s.stepOutputs(recipe.Steps), SourceLocale: recipe.SourceLocale, Locale: recipe.Locale,
		Pictures: []PictureOutput{},
	}
	if recipe.Id != nil {
		output.ID = recipe.Id.Hex()
	}
	for _, id := range recipe.Pictures {
		output.Pictures = append(output.Pictures, PictureOutput{ID: id, URL: s.pictureBase + id})
	}
	return output
}

func (s *Server) previewOutput(recipe models.RecipePreview) RecipeOutput {
	result := RecipeOutput{
		Title: recipe.Title, Description: recipe.Description, Quantity: recipe.Quantity, Kind: recipe.Kind,
		PreparationTime: recipe.PreparationTime, CookingTime: recipe.CookingTime, RestingTime: recipe.RestingTime,
		SourceLocale: recipe.SourceLocale, Locale: recipe.Locale,
		Pictures: []PictureOutput{},
	}
	if recipe.Id != nil {
		result.ID = recipe.Id.Hex()
	}
	for _, id := range recipe.Pictures {
		result.Pictures = append(result.Pictures, PictureOutput{ID: id, URL: s.pictureBase + id})
	}
	return result
}

func (s *Server) list(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
	userID, err := userWithScope(req, "recipes:read")
	if err != nil {
		return nil, ListOutput{}, err
	}
	cursor, err := decodeCursor(input.Cursor)
	if err != nil {
		return nil, ListOutput{}, err
	}
	items, total, next, err := s.recipes.ListForUser(ctx, userID, cursor, input.Limit, input.Locale)
	if err != nil {
		return nil, ListOutput{}, err
	}
	output := ListOutput{Total: total, NextCursor: encodeCursor(next)}
	for _, item := range items {
		output.Items = append(output.Items, s.previewOutput(item))
	}
	return nil, output, nil
}

func (s *Server) get(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, RecipeOutput, error) {
	userID, err := userWithScope(req, "recipes:read")
	if err != nil {
		return nil, RecipeOutput{}, err
	}
	recipe, err := s.recipes.GetForUser(ctx, input.RecipeID, userID, input.Locale)
	if err != nil {
		return nil, RecipeOutput{}, err
	}
	output := s.recipeOutput(recipe)
	data, _ := json.Marshal(output)
	content := make([]mcp.Content, 0, len(output.Pictures)+1)
	content = append(content, &mcp.TextContent{Text: string(data)})
	for _, picture := range output.Pictures {
		content = append(content, &mcp.ResourceLink{URI: picture.URL, Name: picture.ID, Title: recipe.Title + " picture", MIMEType: mediaTypeFromID(picture.ID)})
	}
	return &mcp.CallToolResult{Content: content}, output, nil
}

func mediaTypeFromID(id string) string {
	if strings.HasSuffix(strings.ToLower(id), ".png") {
		return "image/png"
	}
	return "image/jpeg"
}

func (s *Server) create(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, RecipeOutput, error) {
	userID, err := userWithScope(req, "recipes:write")
	if err != nil {
		return nil, RecipeOutput{}, err
	}
	recipe, err := s.recipes.Create(ctx, userID, models.CreateRecipe{
		Title: input.Title, Description: input.Description, Quantity: input.Quantity, Kind: input.Kind,
		PreparationTime: input.PreparationTime, CookingTime: input.CookingTime, RestingTime: input.RestingTime,
		Ingredients: input.Ingredients, Steps: input.Steps, SourceLocale: input.Locale,
	}, nil, nil)
	if err != nil {
		return nil, RecipeOutput{}, err
	}
	return nil, s.recipeOutput(recipe), nil
}

func (s *Server) update(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, RecipeOutput, error) {
	userID, err := userWithScope(req, "recipes:write")
	if err != nil {
		return nil, RecipeOutput{}, err
	}
	canonical, err := s.recipes.GetForUser(ctx, input.RecipeID, userID, "")
	if err != nil {
		return nil, RecipeOutput{}, err
	}
	updated, err := s.recipes.Update(ctx, canonical, models.UpdateRecipeRequest{
		Title: input.Title, Description: input.Description, Quantity: input.Quantity, Kind: input.Kind,
		PreparationTime: input.PreparationTime, CookingTime: input.CookingTime, RestingTime: input.RestingTime,
		Ingredients: input.Ingredients, Steps: input.Steps, Locale: input.Locale, KeepPictureIDs: input.KeepPictureIDs,
	}, nil, nil)
	if err != nil {
		return nil, RecipeOutput{}, err
	}
	return nil, s.recipeOutput(updated), nil
}
