import * as React from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiRequestError, setCsrfToken } from "@/lib/api";
import type { SessionView } from "@/lib/api-types";

/** Session state for the whole app.
 *
 *  One query answers "which screen am I on": the API reports the gate, and the
 *  CSRF token for writes rides along in the same answer. Keeping it in the query
 *  cache means a stale session is refetched rather than trusted, which is what
 *  stops a revoked operator from being shown a live panel. */

interface SessionState {
  session: SessionView | undefined;
  isLoading: boolean;
  error: Error | null;
  refresh: () => Promise<void>;
  signOut: () => Promise<void>;
}

const SessionContext = React.createContext<SessionState | null>(null);

export function useSessionQuery() {
  const query = useQuery({
    queryKey: ["session"],
    queryFn: ({ signal }) => api.session(signal),
    staleTime: 0,
    retry: (count, error) => {
      if (error instanceof ApiRequestError && error.status < 500) return false;
      return count < 2;
    },
  });

  // The CSRF token is only valid for the session that produced it, so it is
  // installed here, once per session answer, and cleared when it goes away.
  const csrf = query.data?.csrf;
  React.useEffect(() => {
    setCsrfToken(csrf ?? null);
  }, [csrf]);

  return query;
}

export function SessionProvider({ children }: { children: React.ReactNode }) {
  const query = useSessionQuery();
  const client = useQueryClient();

  const value = React.useMemo<SessionState>(
    () => ({
      session: query.data,
      isLoading: query.isPending,
      error: (query.error as Error | null) ?? null,
      refresh: async () => {
        await query.refetch();
      },
      signOut: async () => {
        // A failed logout still clears the local view: the operator asked to
        // leave, and hiding the panel is the local half of that.
        try {
          await api.logout();
        } finally {
          setCsrfToken(null);
          client.clear();
        }
      },
    }),
    [query, client],
  );

  return (
    <SessionContext.Provider value={value}>{children}</SessionContext.Provider>
  );
}

export function useSession(): SessionState {
  const context = React.useContext(SessionContext);
  if (!context) throw new Error("useSession 必须在 SessionProvider 内使用");
  return context;
}
