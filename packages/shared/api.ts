export type EntitlementResponse = {
  canTranslate: boolean;
  reason?: string;
  plan: "trial" | "premium" | "none";
  trialActive: boolean;
  weeklyLimitSeconds: number;
  usedSeconds: number;
};

export type QuotaResponse = {
  periodStart: string;
  periodEnd: string;
  weeklyLimitSeconds: number;
  usedSeconds: number;
  remainingSeconds: number;
  usageRefreshTimezone: string;
};

export type CreateTranslationSessionRequest = {
  partyALanguage: string;
  partyBLanguage: string;
  inputMode: "microphone" | "system" | "file";
};

export type RealtimeClientSecretRequest = {
  partyALanguage: string;
  partyBLanguage: string;
};

export type CreateTranslationSessionResponse = {
  id: string;
  status: "created" | "active" | "ended";
  startedAt: string;
};
