import { createContext, useContext, useEffect, useMemo, useState } from "react";

import { ApiError, apiRequest } from "./api";

type Session = {
  username: string;
  expires_at: string;
};

type AuthState =
  | { status: "loading" }
  | { status: "setup-required" }
  | { status: "unauthenticated" }
  | { status: "authenticated"; session: Session }
  | { status: "error"; message: string };

type AuthContextValue = {
  state: AuthState;
  refresh: () => Promise<void>;
  login: (username: string, password: string) => Promise<void>;
  setup: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
};

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [state, setState] = useState<AuthState>({ status: "loading" });

  async function refresh() {
    setState({ status: "loading" });
    try {
      const session = await apiRequest<Session>("/api/v1/auth/me");
      setState({ status: "authenticated", session });
    } catch (error) {
      if (error instanceof ApiError && error.status === 412) {
        setState({ status: "setup-required" });
        return;
      }
      if (error instanceof ApiError && error.status === 401) {
        setState({ status: "unauthenticated" });
        return;
      }
      setState({ status: "error", message: error instanceof Error ? error.message : "unknown error" });
    }
  }

  async function login(username: string, password: string) {
    await apiRequest<{ username: string }>("/api/v1/auth/login", {
      method: "POST",
      body: JSON.stringify({ username, password }),
    });
    await refresh();
  }

  async function setup(username: string, password: string) {
    await apiRequest<{ username: string }>("/api/v1/auth/setup", {
      method: "POST",
      body: JSON.stringify({ username, password }),
    });
    await refresh();
  }

  async function logout() {
    await apiRequest<void>("/api/v1/auth/logout", { method: "POST" });
    setState({ status: "unauthenticated" });
  }

  useEffect(() => {
    void refresh();
  }, []);

  const value = useMemo(() => ({ state, refresh, login, setup, logout }), [state]);
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const value = useContext(AuthContext);
  if (value == null) {
    throw new Error("useAuth must be used inside AuthProvider");
  }
  return value;
}
