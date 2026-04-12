---
name: oura-reader
description: >
  How to fetch and analyze Oura Ring health data (sleep, readiness, activity, heart rate, stress, HRV, SpO2, workouts)
  from an Oura Reader API server. Use this skill whenever the user asks about their health metrics, sleep quality,
  recovery, readiness scores, activity levels, heart rate trends, stress levels, or any biometric data that comes
  from a smart ring. Also trigger when the user mentions "Oura", "ring data", "sleep score", "readiness score",
  or wants to look at trends in their health data over time.
---

# Oura Reader API Skill

You have access to an Oura Reader server — a service that syncs and caches Oura Ring health data and serves it
over a simple REST API. This skill teaches you how to connect to it and retrieve health data for analysis.

## Configuration

Before making any API calls, resolve the server URL and API key. Check in this order, stopping at the first
source that provides values:

1. **Environment variables**: `$OURA_READER_URL` and `$OURA_READER_API_KEY`
2. **Config file**: `~/.config/oura-reader/config.json` (format: `{"url": "...", "api_key": "..."}`)
3. **Ask the user**: prompt for both, then save to `~/.config/oura-reader/config.json` for future sessions

Verify connectivity: `curl -s "$URL/api/v1/health"` — expected: `{"status":"ok"}`

## Authentication

All API calls (except `/api/v1/health`) require a Bearer token:

```
Authorization: Bearer <API_KEY>
```

## Complete API Reference

### Health & Auth

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| GET | `/api/v1/health` | No | Health check — returns `{"status":"ok"}` |
| GET | `/api/v1/auth/login` | Yes | Initiates OAuth flow with Oura (redirects to Oura login) |
| GET | `/api/v1/auth/callback` | No | OAuth callback (browser redirect, not called directly) |
| GET | `/api/v1/auth/status` | Yes | Check if OAuth token is valid — returns `{"authenticated": bool, "token_expiry": "..."}` |

### Sync

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| POST | `/api/v1/sync` | Yes | Trigger full sync of all endpoints from Oura API |
| POST | `/api/v1/sync/{endpoint}` | Yes | Sync a single endpoint (e.g., `/api/v1/sync/daily_sleep`) |
| GET | `/api/v1/sync/status` | Yes | Last sync date per endpoint |

The server auto-syncs every ~6 hours. Manual sync is only needed for the very latest data.
Sync is incremental — fetches from last sync date to today.

### Data

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| GET | `/api/v1/data/{endpoint}` | Yes | Fetch data for one endpoint |
| GET | `/api/v1/data` | Yes | Fetch data from all endpoints at once |

**Query parameters** (both routes):

| Param | Format | Default | Notes |
|-------|--------|---------|-------|
| `start_date` | `YYYY-MM-DD` | — | Filter start (inclusive) |
| `end_date` | `YYYY-MM-DD` | — | Filter end (inclusive) |
| `limit` | integer | 100 | Max records returned |
| `offset` | integer | 0 | For pagination (single-endpoint only) |

**Single-endpoint response:**
```json
{"data": [...], "count": 42, "limit": 100, "offset": 0}
```
If `count > limit`, paginate by increasing `offset`.

**All-endpoints response:**
```json
{"daily_sleep": [...], "sleep": [...], "daily_readiness": [...], ...}
```
A JSON object keyed by endpoint name, each containing an array of records.
Use this for broad queries like "full health check" — one call instead of many.

## Available Data Endpoints

All 18 endpoints below are valid values for `{endpoint}` in the data and sync routes.

### Sleep & Recovery
| Endpoint | What it contains | Date-filtered |
|----------|-----------------|:---:|
| `daily_sleep` | Nightly sleep score (0-100), contributors (efficiency, latency, timing, deep sleep, REM, restfulness, total sleep) | Yes |
| `sleep` | Detailed sleep sessions — stages (deep, REM, light, awake), duration, HR, HRV, respiratory rate, bedtime/wake time, efficiency, latency | Yes |
| `sleep_time` | Recommended bedtime window and sleep duration targets | Yes |
| `daily_readiness` | Readiness score (0-100), contributors (resting HR, HRV balance, recovery index, body temperature, sleep balance, activity balance) | Yes |

### Activity & Fitness
| Endpoint | What it contains | Date-filtered |
|----------|-----------------|:---:|
| `daily_activity` | Activity score, steps, calories (active + total), active/sedentary time, movement breakdown (high/medium/low) | Yes |
| `workout` | Detected workouts — type, duration, calories, average/max HR, intensity, distance | Yes |
| `session` | Guided breathing / meditation sessions — type, duration, HR, HRV, mood | Yes |
| `vo2_max` | VO2 max estimates over time | Yes |

### Vitals & Biometrics
| Endpoint | What it contains | Date-filtered |
|----------|-----------------|:---:|
| `heartrate` | 5-minute heart rate samples throughout the day. **Warning: voluminous** — limit to 1-2 day ranges | Yes |
| `daily_spo2` | Blood oxygen saturation averages | Yes |
| `daily_cardiovascular_age` | Estimated cardiovascular age | Yes |

### Stress & Resilience
| Endpoint | What it contains | Date-filtered |
|----------|-----------------|:---:|
| `daily_stress` | Daily stress summary — time in high/medium/low/rest stress states | Yes |
| `daily_resilience` | Resilience score and contributors | Yes |

### Tags & Annotations
| Endpoint | What it contains | Date-filtered |
|----------|-----------------|:---:|
| `tag` | User-created tags (e.g., "caffeine", "alcohol", "late_meal") | Yes |
| `enhanced_tag` | Tags with additional context and metadata | Yes |

### Other
| Endpoint | What it contains | Date-filtered |
|----------|-----------------|:---:|
| `rest_mode_period` | Rest mode periods (user-activated recovery mode) | Yes |
| `ring_configuration` | Ring hardware info — model, color, size, firmware | No |
| `personal_info` | User profile — age, height, weight, biological sex. Returns a single object, not a list | No |

## Common Patterns

### "How did I sleep last night?"

Fetch `daily_sleep` for the score and `sleep` for the detailed breakdown. If the user mentions
feeling tired/groggy, also pull `daily_readiness` to explain recovery state.

### "How's my recovery / readiness?"

Fetch `daily_readiness` — the score reflects recovery. Cross-reference with `daily_sleep`
(poor sleep = lower readiness) and `daily_activity` (high strain = lower readiness).

### "Full health check" / broad health question

Use the bulk endpoint `GET /api/v1/data?start_date=...&end_date=...` to get everything in one call.
Focus analysis on `daily_sleep`, `daily_readiness`, `daily_activity`, and `daily_stress`. Pull in
`workout`, `daily_spo2`, and `daily_cardiovascular_age` for a complete picture.

### "Show me trends over the past week/month"

Use date ranges to pull multiple days. Calculate averages, spot patterns, flag outliers.
Best endpoints for trends: `daily_sleep`, `daily_readiness`, `daily_activity`, `daily_stress`.

### "Heart rate analysis"

The `heartrate` endpoint returns 5-minute samples — can be very large. Always use narrow date ranges
(1-2 days). Look for resting HR (overnight minimum), daytime patterns, exercise spikes.

### "Am I stressed?"

Combine `daily_stress` (time in stress states) with `daily_resilience` (capacity to handle stress)
and `heartrate` (elevated resting HR = physiological stress).

### Training / workout context

When the user mentions exercise, gym, or training, pull `workout` alongside recovery endpoints
(`daily_readiness`, `daily_stress`, `daily_resilience`) to correlate training load with recovery.

## Interpreting Oura Scores

- **Scores are 0-100**: 85+ is excellent, 70-84 is good, below 70 suggests attention needed
- **Contributors**: Each score has contributor breakdowns explaining *why* a score is high or low
- **Trends matter more than single days**: One bad night isn't concerning; a downward trend over a week is
- **Cross-reference**: Sleep with activity, readiness with stress — the data tells a story together
- **Personal baselines vary**: Compare to the user's own history, not population averages

## Tips

- For vague health questions, default to the last 7 days of the most relevant endpoints
- Always mention the date range you're analyzing so the user knows the scope
- If data is missing or empty for a date range, the user may not have worn their ring — mention this
- For a quick holistic view, use `GET /api/v1/data` instead of calling endpoints individually
- Check `personal_info` for age/weight/height context when interpreting cardiovascular or fitness metrics
