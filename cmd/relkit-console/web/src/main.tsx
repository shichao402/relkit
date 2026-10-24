import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router-dom";
import { Toaster } from "sonner";
import { App } from "./app";
import "./styles.css";

// The app is mounted under /-/admin/ so deep links work without a server-side
// route table; BrowserRouter only needs to know the prefix.
const basename = "/-/admin";

const client = new QueryClient({
  defaultOptions: {
    queries: {
      // The panel shows operational state: a value cached from minutes ago is a
      // lie. Background refetch keeps a visible page honest without a spinner.
      staleTime: 5_000,
      refetchOnWindowFocus: true,
      retry: 1,
    },
    mutations: {
      // A write must never be retried automatically: "rotate token" fired twice
      // is the very failure acceptance finding #3 was about.
      retry: 0,
    },
  },
});

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={client}>
      <BrowserRouter basename={basename}>
        <App />
        <Toaster position="top-center" richColors closeButton />
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
);
