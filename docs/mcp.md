# Recipes MCP server

The API exposes a stateless Streamable HTTP MCP server at `/mcp`. Point a remote MCP-capable agent at the public URL; for production that is:

```text
https://recipes.suolumi.fr/mcp
```

The agent discovers the OAuth authorization server, opens the Recipes login/permission page, and uses OAuth Authorization Code with PKCE S256. Access tokens are specific to MCP and cannot be used as REST JWTs. MCP access tokens last one hour; rotating refresh sessions last 30 days. Consent is requested on every authorization flow. Changing the account password or deleting the account invalidates its MCP access.

The server implements the current `2026-07-28` MCP protocol only. It uses the official Go MCP SDK and has no admin-specific behavior: every account, including an admin account, can access only recipes it authored through MCP.

## Configuration

Set these environment variables in production:

```text
RECIPES_MCP_PUBLICURL=https://recipes.suolumi.fr/mcp
RECIPES_MCP_JWTSECRET=<at-least-32-random-bytes>
```

`RECIPES_MCP_PUBLICURL` must use HTTPS outside localhost and must have exactly the `/mcp` path. In local development it defaults to `http://localhost:<RECIPES_CFG_PORT>/mcp`. If the MCP secret is omitted locally, the process generates an ephemeral secret and existing MCP tokens stop working after restart.

Optional settings and defaults:

| Variable | Default |
| --- | --- |
| `RECIPES_MCP_ACCESSEXPIRATION` | `1h` |
| `RECIPES_MCP_REFRESHEXPIRATION` | `720h` |
| `RECIPES_MCP_AUTHORIZATIONCODETTL` | `10m` |
| `RECIPES_MCP_IDEMPOTENCYTTL` | `24h` |
| `RECIPES_MCP_MAXDECODEDPICTUREBYTES` | `67108864` |

OAuth authorization codes, rotating refresh sessions, and idempotency results are persisted in MongoDB. Consent grants are not persisted. MCP access-token validation itself is stateless apart from checking that the user still exists and has not changed their password.

## OAuth client requirements

Clients use an HTTPS Client ID Metadata Document URL as their `client_id`. Local development permits an HTTP localhost metadata document, and HTTP loopback callbacks are accepted for native agents. The document must contain matching `client_id`, a non-empty `client_name`, and at least one `redirect_uri`; it must describe a public client using `token_endpoint_auth_method: "none"`, authorization code, and the `code` response type. Dynamic client registration and confidential-client authentication are not enabled.

Clients that can store rotating refresh tokens should also include `refresh_token` in their metadata document's `grant_types`; otherwise the authorization server issues only an access token.

Discovery endpoints:

```text
/.well-known/oauth-protected-resource/mcp
/.well-known/oauth-authorization-server
```

Available scopes are `recipes:read` and `recipes:write`.

## Tools

- `list_my_recipes`: accepts optional `cursor`, `limit` (default 20, maximum 100), and one BCP 47 `locale`. It returns the total, items, and an opaque next cursor.
- `get_my_recipe`: accepts `recipe_id` and optional `locale`. Pictures contain both their stored ID and public URL, and are also returned as MCP resource links.
- `create_recipe`: requires `idempotency_key`, a complete recipe, and optional `pictures` in the same call.
- `update_recipe`: requires `idempotency_key` and `recipe_id`; all recipe fields are true patch fields. Optional `keep_picture_ids` gives the ordered existing pictures to retain. New pictures are appended in request order.

There is intentionally no MCP delete tool. Recipe deletion remains available through the website/REST API.

Each MCP picture is an object with `filename`, `media_type`, and standard base64 `data`. JPEG and PNG are accepted. The decoded total across a call is limited to 64 MiB; each image is limited to 40 megapixels and 16,384 pixels on either axis. Content, declared media type, and filename extension must agree. Images are auto-oriented and re-encoded in their original format, which strips EXIF/GPS metadata. Zero pictures is valid.

Recipes must have a nonblank title, quantity of at least one, a supported kind, nonnegative times, at least one named ingredient, and at least one step with a description. Incomplete recipes are rejected rather than saved as drafts.

## Localization

`list_my_recipes` and `get_my_recipe` accept only one locale. A fresh stored translation is returned when present. Missing or stale translations are translated and replaced in MongoDB. If translation fails, that individual recipe falls back to canonical, and the returned `locale` and `source_locale` identify what was actually returned.

## REST picture creation/update

REST recipe creation and patch now accept either ordinary `application/json` (no newly uploaded pictures) or `multipart/form-data`. Multipart requests contain a `recipe` field with the JSON recipe/patch and zero or more repeated `pictures` file fields. On patch, omitted `keep_picture_ids` leaves existing pictures unchanged, while an empty array removes all of them. The old standalone recipe-picture upload/delete endpoints have been removed; public picture GET remains at `/api/v1/recipe-pictures/{id}`.

REST clients may send `Idempotency-Key` on recipe creation or patch. Replays with the same user, operation, key, and input return the saved response; reuse with different input returns `409 Conflict`.
