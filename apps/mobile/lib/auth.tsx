import { createContext, useContext, useMemo, useState } from "react";
import type { ApiUser, AuthResponse } from "./api";

type AuthState = {
  token: string | null;
  user: ApiUser | null;
  setSession: (session: AuthResponse) => void;
  signOut: () => void;
};

const AuthContext = createContext<AuthState | null>(null);

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [token, setToken] = useState<string | null>(null);
  const [user, setUser] = useState<ApiUser | null>(null);

  const value = useMemo(
    () => ({
      token,
      user,
      setSession(session: AuthResponse) {
        setToken(session.token);
        setUser(session.user);
      },
      signOut() {
        setToken(null);
        setUser(null);
      }
    }),
    [token, user]
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const value = useContext(AuthContext);
  if (!value) {
    throw new Error("useAuth must be used inside AuthProvider");
  }
  return value;
}
