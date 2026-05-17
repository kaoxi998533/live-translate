# Live Translate Platform

Commercial-grade real-time translation platform.

## Stack

- Mobile: Expo, React Native, TypeScript
- API: Go
- Data: PostgreSQL, Redis
- Billing: Stripe Billing

## Apps

- `apps/mobile`: customer-facing iOS and Android app
- `apps/api`: Go API server

## Local Development

```sh
docker compose up -d
npm install
OPENAI_API_KEY=sk-proj-your-key npm run dev:api
npm run android:mobile
```

Android is the priority target. Because the app uses `react-native-webrtc` to
connect directly to OpenAI Realtime, it must run in an Android development build
or a native Android build. Expo Go is not enough for this native module.

For the Android emulator, the mobile app defaults to `http://10.0.2.2:8080` for
the Go API. Override it with `EXPO_PUBLIC_API_BASE_URL` when testing on a real
device.

## Product Model

Translation access is controlled by server-side entitlements:

- Registered users can receive a short trial.
- Premium access requires an active subscription.
- Premium users still have weekly translation limits.
- Usage is recorded as ledger events for auditability.

## Realtime Interpretation

The mobile app asks the Go API for an OpenAI Realtime client secret. The Go API
uses `OPENAI_API_KEY` server-side, so the main API key is never shipped to the
phone. The Android app then connects to `gpt-realtime` over WebRTC and streams
microphone audio directly to OpenAI Realtime.

The product is designed for two-person conversations. The user chooses Person
A's language and Person B's language; the model infers which of those two
languages is currently being spoken and outputs the translation for the other
person.

The Realtime defaults intentionally reuse the latency settings proven in the
prototype project: server VAD, 450 ms silence duration, 250 ms prefix padding,
`marin` voice, and input transcription for the on-screen source transcript.
The commercial app exposes these as backend configuration rather than regular
user-facing controls.
