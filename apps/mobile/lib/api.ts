export const API_BASE_URL =
  process.env.EXPO_PUBLIC_API_BASE_URL ?? "http://10.0.2.2:8080";

export type ApiUser = {
  id: string;
  email: string;
  displayName?: string;
  plan: "trial" | "premium";
  trialEndsAt: string;
  createdAt: string;
};

export type AuthResponse = {
  token: string;
  user: ApiUser;
};

export type Entitlement = {
  canTranslate: boolean;
  reason?: string;
  plan: "trial" | "premium";
  trialActive: boolean;
  weeklyLimitSeconds: number;
  usedSeconds: number;
  remainingSeconds: number;
};

export type TranslationSession = {
  id: string;
  status: "active" | "ended";
  startedAt: string;
  remainingSeconds: number;
};

export type RealtimeClientSecret = {
  value: string;
  expiresAt?: number;
  model: string;
};

export type PaymentOrder = {
  id: string;
  planId: "premium";
  provider: "wechat_pay" | "stripe" | "apple_iap" | "google_play" | "alipay";
  providerOrderId: string;
  amountMinor: number;
  currency: string;
  status: "pending" | "paid" | "failed" | "canceled";
  createdAt: string;
};

export async function register(email: string, password: string) {
  return request<AuthResponse>("/v1/auth/register", {
    method: "POST",
    body: { email, password }
  });
}

export async function login(email: string, password: string) {
  return request<AuthResponse>("/v1/auth/login", {
    method: "POST",
    body: { email, password }
  });
}

export async function getEntitlement(token: string) {
  return request<Entitlement>("/v1/entitlements", { token });
}

export async function createTranslationSession(
  token: string,
  partyALanguage: string,
  partyBLanguage: string
) {
  return request<TranslationSession>("/v1/translation/sessions", {
    method: "POST",
    token,
    body: {
      partyALanguage,
      partyBLanguage,
      inputMode: "microphone"
    }
  });
}

export async function createRealtimeClientSecret(
  token: string,
  partyALanguage: string,
  partyBLanguage: string,
  listenMode: "auto" | "hold"
) {
  return request<RealtimeClientSecret>("/v1/realtime/client-secret", {
    method: "POST",
    token,
    body: { partyALanguage, partyBLanguage, listenMode }
  });
}

export async function addUsage(token: string, sessionId: string, seconds: number) {
  return request<Entitlement>(`/v1/translation/sessions/${sessionId}/usage`, {
    method: "POST",
    token,
    body: { seconds }
  });
}

export async function endTranslationSession(token: string, sessionId: string) {
  return request(`/v1/translation/sessions/${sessionId}/end`, {
    method: "POST",
    token
  });
}

export async function createPaymentOrder(token: string, provider = "wechat_pay") {
  return request<PaymentOrder>("/v1/billing/orders", {
    method: "POST",
    token,
    body: { planId: "premium", provider }
  });
}

export async function markPaymentOrderPaidForDev(token: string, orderId: string) {
  return request<ApiUser>(`/v1/dev/billing/orders/${orderId}/mark-paid`, {
    method: "POST",
    token
  });
}

async function request<T>(
  path: string,
  options: {
    method?: "GET" | "POST";
    token?: string;
    body?: unknown;
  } = {}
): Promise<T> {
  const response = await fetch(`${API_BASE_URL}${path}`, {
    method: options.method ?? "GET",
    headers: {
      "Content-Type": "application/json",
      ...(options.token ? { Authorization: `Bearer ${options.token}` } : {})
    },
    body: options.body ? JSON.stringify(options.body) : undefined
  });

  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    const message =
      typeof payload.error === "string"
        ? payload.error
        : `请求失败：${response.status}`;
    throw new Error(message);
  }

  return payload as T;
}
