# WearWhere Backend (Go)

Backend for the WearWhere project — currently implementing the **Authentication & Profile** module per SRS section 3.1 (UC03–UC09) and section 4.2 Security NFRs.

## Stack

| Layer        | Tech                                  |
| ------------ | ------------------------------------- |
| Language     | Go 1.22                               |
| HTTP         | Gin                                   |
| Database     | PostgreSQL 16 (pgx/v5)                |
| Cache / OTP  | Redis 7                               |
| Auth         | JWT (HS256) + opaque refresh tokens   |
| Email        | SMTP (gomail)                         |
| SMS          | Twilio                                |
| Validation   | go-playground/validator              |

## Folder layout

```
cmd/api/             entrypoint
internal/
  auth/              this feature: handler / service / repo / domain / middleware
  config/            env loader
  shared/            cross-cutting: postgres, redis, jwt, hash, mailer, sms, validator
pkg/httpx/           HTTP helpers (response envelope, AppError)
db/migrations/       SQL migrations (golang-migrate format)
```

## Endpoints

All under `/api/v1`. Public unless noted.

| Method | Path                            | Description                              |
| ------ | ------------------------------- | ---------------------------------------- |
| POST   | `/auth/register`                | UC03 — register with email or phone      |
| POST   | `/auth/login`                   | UC04 — login (email / phone + password)  |
| POST   | `/auth/refresh`                 | rotate access + refresh tokens           |
| POST   | `/auth/logout`                  | UC05 — requires Bearer                   |
| POST   | `/auth/password/forgot`         | UC06 step 1 — request OTP                |
| POST   | `/auth/password/reset`          | UC06 step 2 — verify OTP + new password  |
| POST   | `/auth/otp/send`                | resend verify/reset OTP                  |
| POST   | `/auth/otp/verify`              | verify email/phone OTP                   |
| POST   | `/auth/oauth/google`            | sign in / up via Google ID token         |
| POST   | `/auth/oauth/apple`             | sign in / up via Apple identity token    |
| POST   | `/auth/brand/login`             | UC41 — Brand Portal login (rejects non-brand) |
| POST   | `/auth/admin/login`             | UC52 — Admin CMS login (rejects non-admin)    |
| GET    | `/me`                           | current user profile (Bearer)            |
| PATCH  | `/me`                           | UC07 — edit profile (Bearer)             |
| POST   | `/me/password`                  | UC08 — change password (Bearer)          |
| DELETE | `/me`                           | UC09 — soft delete account (Bearer)      |

### Response envelopes

Success:
```json
{ "user": { "...": "..." }, "tokens": { "access_token": "...", "refresh_token": "...", "expires_at": "..." } }
```
Error:
```json
{ "error": { "code": "INVALID_CREDENTIALS", "message": "..." } }
```

## Getting started

### 1. Prerequisites

- Go 1.22+
- Docker (for postgres + redis) — optional if you already have them locally
- `golang-migrate` CLI:
  ```bash
  go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
  ```

### 2. Configure

```bash
cp .env.example .env
# open .env and fill in real values (see SMTP / Twilio / OAuth sections below)
```

Generate a strong JWT secret:
```bash
openssl rand -base64 64
```

### 3. Start dependencies

```bash
make up               # docker-compose: postgres on 5432, redis on 6379
```

### 4. Run migrations

```bash
make migrate-up
```

### 5. Run the API

```bash
go mod tidy
make run              # → http://localhost:8080/healthz
```

## External services — how to set up

### A. SMTP (for email OTP)

The fastest path is a Gmail App Password:

1. Enable 2-factor auth on your Google account.
2. https://myaccount.google.com/apppasswords → create an app password for "Mail".
3. In `.env`:
   ```
   SMTP_HOST=smtp.gmail.com
   SMTP_PORT=587
   SMTP_USERNAME=your.email@gmail.com
   SMTP_PASSWORD=<16-char app password>
   SMTP_FROM_EMAIL=your.email@gmail.com
   ```

Alternatives: SendGrid (free 100/day), Mailtrap (dev sandbox), Resend.

If `SMTP_HOST` is empty, emails fall back to **stdout** so the OTP is still visible during development.

### B. Twilio (for SMS OTP)

1. Sign up at https://www.twilio.com — free trial includes ~$15 credit.
2. From the console copy `Account SID` and `Auth Token`.
3. Get a Twilio phone number (free trial number works; only verified numbers can receive SMS until you upgrade).
4. In `.env`:
   ```
   TWILIO_ACCOUNT_SID=ACxxxxxxxx
   TWILIO_AUTH_TOKEN=xxxxxxxx
   TWILIO_FROM_NUMBER=+15551234567
   ```

If `TWILIO_ACCOUNT_SID` is empty, SMS falls back to **stdout** (dev mode).

### C. Google Sign-In *(optional — can defer)*

Until you fill `GOOGLE_CLIENT_IDS` in `.env`, the `/auth/oauth/google` endpoint stays mounted but rejects any call with `SOCIAL_TOKEN_INVALID`. Frontend should not call it yet.

When you're ready: create one OAuth Client ID **per platform** in Google Cloud Console → APIs & Services → Credentials:

| Platform     | Type    | Notes                                                         |
| ------------ | ------- | ------------------------------------------------------------- |
| Next.js web  | Web     | Authorised origins + redirect URIs                            |
| Flutter iOS  | iOS     | Bundle ID                                                     |
| Flutter Android | Android | Package name + debug & release SHA-1 fingerprints          |

Audience matrix the backend will see:

| Token source          | `aud` claim          |
| --------------------- | -------------------- |
| Web                   | **Web Client ID**    |
| Android (via `serverClientId=<web>`) | **Web Client ID** |
| iOS                   | **iOS Client ID**    |

→ Configure both as a comma-separated list:
```
GOOGLE_CLIENT_IDS=xxx-web.apps.googleusercontent.com,yyy-ios.apps.googleusercontent.com
```

Frontend obtains the Google **ID token** and POSTs it as `{"id_token":"..."}` to `/api/v1/auth/oauth/google`. The backend verifies it via Google's `tokeninfo` endpoint and rejects the token if `aud` is not in `GOOGLE_CLIENT_IDS`.

### D. Apple Sign-In *(optional — can defer)*

Required on iOS per App Store guideline 4.8 when other social logins are present. Until you fill `APPLE_CLIENT_IDS` in `.env`, the `/auth/oauth/apple` endpoint stays mounted but rejects any call with `SOCIAL_TOKEN_INVALID`.

1. https://developer.apple.com/account/resources/identifiers
   - Register a **Services ID** (e.g. `com.wearwhere.app.signin`) — used by web + Android (Android takes the web OAuth flow).
   - Register / configure your **App ID** (e.g. `com.wearwhere.app`) — used by native iOS sign-in. Enable "Sign In with Apple" capability.
2. Configure your domain + return URLs on the Services ID.
3. In `.env` — list both because the `aud` in the ID token differs by platform:
   ```
   APPLE_CLIENT_IDS=com.wearwhere.app.signin,com.wearwhere.app
   ```

The frontend (iOS / web) obtains an **identity token** from Apple and POSTs it as `{"id_token":"<jwt>"}` to `/api/v1/auth/oauth/apple`. The backend (`internal/shared/apple`) fetches Apple's JWKs from `https://appleid.apple.com/auth/keys` (cached 15 min), verifies the RS256 signature, and validates `iss=https://appleid.apple.com`, `aud=APPLE_CLIENT_ID`, and `exp`.

> Apple only returns the user's name on the **first** sign-in (via separate fields the iOS SDK passes back) — the ID token does not contain it. Persist the name on registration; subsequent logins match by the stable `sub` claim.

### E. Role-gated portals

- **UC41 Brand Portal:** `POST /api/v1/auth/brand/login` — same payload as `/auth/login` but the response is rejected if the account's role is not `brand`.
- **UC52 Admin CMS:** `POST /api/v1/auth/admin/login` — same, restricted to `admin`.

To create admin/brand users, insert directly into the DB (or use a future admin CLI):
```sql
UPDATE users SET role='admin' WHERE email='admin@wearwhere.vn';
```

## Security notes

- Passwords hashed with **bcrypt cost 12** (SRS 4.2 NFR-11).
- Access tokens: 15 min default. Refresh tokens: 30 days, opaque, stored as SHA-256 hash in DB, rotated on every refresh (NFR-12).
- Brute force: 5 failed attempts in 15 min → 15 min lockout (NFR-16).
- Rate limit: 100 req/min/user (NFR-18) via Redis fixed-window.
- TLS termination expected at reverse proxy (NFR-14).
- Account deletion: soft delete + 90-day purge (UC09, NFR-25).

## Testing

```bash
make test
```

Curl quick check:
```bash
# register
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"a@b.com","password":"Secret1!","name":"Alice"}'

# login
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"a@b.com","password":"Secret1!"}'

# me
curl http://localhost:8080/api/v1/me -H 'Authorization: Bearer <access_token>'
```
