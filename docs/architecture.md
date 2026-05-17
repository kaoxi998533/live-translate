# Architecture

Live Translate Platform is split around product control boundaries.

## Mobile

The Expo React Native app owns customer interaction:

- Authentication screens
- Translation workspace
- Usage dashboard
- Billing entry points

The mobile app should stay thin: it displays entitlement state and captures
audio, while the API remains responsible for authorization, session creation,
and quota enforcement.

## API

The Go API owns commercial enforcement:

- JWT verification
- Subscription state
- Trial grants
- Weekly quota checks
- Translation session lifecycle
- Usage ledger writes

## Entitlement Flow

1. The client requests translation access.
2. The API verifies the user.
3. The API checks active subscription, active trial, and weekly quota.
4. The API creates a translation session only if access is allowed.
5. During translation, the API records usage events and updates the current quota period.

The client may display remaining usage, but the server is the source of truth.

## Realtime Translation Flow

1. The Android app creates a platform translation session through the Go API.
2. The Go API checks trial, subscription, and weekly quota.
3. The Android app requests an OpenAI Realtime client secret from the Go API.
4. The Go API calls `POST /v1/realtime/client_secrets` with `OPENAI_API_KEY`.
5. The Android app uses the ephemeral client secret to connect to OpenAI
   Realtime over WebRTC.
6. The app periodically reports usage to the Go API while the Realtime session
   is active.
