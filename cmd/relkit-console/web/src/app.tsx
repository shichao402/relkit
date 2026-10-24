import * as React from "react";
import {
  Navigate,
  Route,
  Routes,
  useLocation,
} from "react-router-dom";
import { SessionProvider, useSession } from "@/lib/session";
import { AppShell } from "@/components/app-shell";
import { BlockedPage, SetupPage, SignInPage } from "@/pages/auth";
import { Skeleton } from "@/components/ui/skeleton";
import { ProductsPage } from "@/pages/products";
import { ProductPage } from "@/pages/product-detail";
import { ProductTokensPage } from "@/pages/product-tokens";
import { UsersPage } from "@/pages/users";
import { RolesPage } from "@/pages/roles";
import { TokensPage } from "@/pages/tokens";
import { FilesPage } from "@/pages/files";

/** Route table.
 *
 *  Every destination in navItems has an element here, and the navigation test in
 *  src/app.nav.test.tsx asserts it: a sidebar entry with no route is exactly the
 *  broken-link class of bug acceptance finding #1 reported. */

export function App() {
  return (
    <SessionProvider>
      <Gate />
    </SessionProvider>
  );
}

function Gate() {
  const { session, isLoading, error } = useSession();
  const location = useLocation();

  if (isLoading) {
    return (
      <div className="mx-auto max-w-md space-y-3 px-4 py-24">
        <Skeleton className="h-6 w-40" />
        <Skeleton className="h-24 w-full" />
      </div>
    );
  }

  if (error) {
    return (
      <BlockedPage
        kind="unavailable"
        detail={`${error.message}（页面可重试）`}
      />
    );
  }

  if (!session || session.gate === "unavailable") {
    return <BlockedPage kind="unavailable" detail={session?.agent} />;
  }
  if (session.gate === "locked") return <BlockedPage kind="locked" />;
  if (session.gate === "setup") return <SetupPage />;
  if (session.gate === "login") {
    if (location.pathname === "/sign-in") return <SignInPage />;
    const next = encodeURIComponent(location.pathname + location.search);
    return <Navigate to={`/sign-in?next=${next}`} replace />;
  }

  // gate === "ready": the operator is signed in. A stale view is still refetched
  // on focus, so a session revoked elsewhere is noticed without a reload.
  return (
    <AppShell>
      <Routes>
        <Route path="/" element={<Navigate to="/products" replace />} />
        <Route path="/products" element={<ProductsPage />} />
        <Route path="/products/:id" element={<ProductPage />} />
        <Route path="/products/:id/tokens" element={<ProductTokensPage />} />
        <Route path="/users" element={<UsersPage />} />
        <Route path="/roles" element={<RolesPage />} />
        <Route path="/tokens" element={<TokensPage />} />
        <Route path="/files" element={<FilesPage />} />
        <Route
          path="*"
          element={
            <Navigate
              to="/products"
              replace
            />
          }
        />
      </Routes>
      <RefreshOnFocus />
    </AppShell>
  );
}

/** Refetching the session when the tab regains focus is what makes a logout in
 *  another tab take effect here without polling. */
function RefreshOnFocus() {
  const { refresh } = useSession();
  React.useEffect(() => {
    const onFocus = () => {
      void refresh();
    };
    window.addEventListener("focus", onFocus);
    return () => window.removeEventListener("focus", onFocus);
  }, [refresh]);
  return null;
}
