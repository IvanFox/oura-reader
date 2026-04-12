# Deployment Guide

Deploy oura-reader behind Tailscale with HTTPS and connect it to the Oura API via OAuth2.

## Prerequisites

- **Oura Ring** with an active Oura membership
- **Tailscale** installed on the server, joined to your tailnet
- **Docker** and Docker Compose installed on the server

## 1. Register an Oura developer app

Go to [cloud.ouraring.com/oauth/applications](https://cloud.ouraring.com/oauth/applications) and create a new application.

| Field | Value |
|-------|-------|
| Display Name | Any name (e.g. "oura-reader") |
| Description | Any description |
| Contact Email | Your email |
| Website | Your GitHub repo URL |
| Privacy Policy | Raw GitHub URL to `PRIVACY.md` (e.g. `https://github.com/<user>/oura-reader/blob/main/PRIVACY.md`) |
| Terms of Service | Raw GitHub URL to `TERMS.md` |
| Redirect URI | `https://<your-tailscale-hostname>/api/v1/auth/callback` |
| Scopes | Select all |

Save the **Client ID** and **Client Secret**.

> **Note:** The app may start in "development" mode. Check if there is an approval step or status toggle in the Oura portal before attempting the OAuth flow.

## 2. Configure Tailscale HTTPS

Enable HTTPS certificates in the [Tailscale admin console](https://login.tailscale.com/admin/dns) (DNS section, enable HTTPS).

Set up `tailscale serve` to proxy HTTPS traffic to the app:

```bash
tailscale serve https / http://localhost:8080
```

Verify the proxy is active:

```bash
tailscale serve status
```

Note your full Tailscale hostname (e.g. `your-server.tail1234.ts.net`).

## 3. Create secrets

```bash
mkdir -p secrets

# Paste your Client ID and Client Secret from Step 1
echo "your_client_id" > secrets/oura_client_id
echo "your_client_secret" > secrets/oura_client_secret

# Generate an encryption key for token storage
openssl rand -hex 32 > secrets/oura_encryption_key

# Lock down permissions
chmod 600 secrets/*
```

## 4. Configure environment

Edit `docker-compose.yml` and uncomment the `OURA_BASE_URL` line, setting it to your Tailscale hostname:

```yaml
environment:
  - OURA_LISTEN_ADDR=0.0.0.0:8080
  - OURA_FETCH_INTERVAL=6h
  - OURA_BASE_URL=https://your-server.tail1234.ts.net
```

The `OURA_BASE_URL` must exactly match the redirect URI registered in Step 1 (minus the `/api/v1/auth/callback` path).

## 5. Deploy

```bash
make docker && make docker-run
```

Verify the container is running:

```bash
docker compose ps
docker compose logs -f oura-reader
```

## 6. Create a user and authorize with Oura

Create a user account:

```bash
docker exec oura-reader-oura-reader-1 oura-reader user add --name "YourName"
```

Save the printed API key — it cannot be retrieved later.

Start the OAuth flow (from a device on your Tailscale network):

```bash
curl -v -H "Authorization: Bearer oura_ak_..." \
  https://your-server.tail1234.ts.net/api/v1/auth/login
```

The response is a redirect (HTTP 302) to Oura's consent page. Open the `Location` URL in a browser, authorize the app, and Oura redirects back to the callback URL. The server exchanges the code for tokens and stores them encrypted.

Verify the token was stored:

```bash
curl -H "Authorization: Bearer oura_ak_..." \
  https://your-server.tail1234.ts.net/api/v1/auth/status
```

## 7. Verify data sync

The scheduler runs automatically at the configured interval (default: every 6 hours). Check sync status:

```bash
curl -H "Authorization: Bearer oura_ak_..." \
  https://your-server.tail1234.ts.net/api/v1/sync/status
```

To trigger an immediate sync, restart the container (the scheduler syncs on startup):

```bash
docker compose restart
```

## 8. Connect an MCP client (optional)

Once the server is running and a user has completed OAuth, you can expose all 18 Oura data endpoints to AI agents via the Model Context Protocol.

See [`clients/mcp/README.md`](../clients/mcp/README.md) for install and client-configuration instructions (Claude Desktop, Claude Code, Cursor).

## Troubleshooting

- **"redirect_uri mismatch" from Oura**: The `OURA_BASE_URL` does not match the redirect URI registered in the Oura portal. Check protocol (`https://`), hostname, and ensure there is no trailing slash.
- **OAuth callback unreachable**: The authorization must happen from a device on your Tailscale network. The `.ts.net` hostname only resolves within the tailnet.
- **Container won't start**: Check `docker compose logs`. Missing secrets produce clear error messages naming the missing file/variable.
