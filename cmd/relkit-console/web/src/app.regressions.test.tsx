import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { App } from "./app";

/** Regressions for the three acceptance findings. Each test names the defect it
 *  guards, so a future failure points at the behaviour rather than at a selector.
 */

const readySession = {
  ok: true,
  gate: "ready",
  user: { username: "operator", role: "admin", createdAt: "2026-10-01T00:00:00Z" },
  csrf: "csrf-token",
  version: "test",
  agent: "http://127.0.0.1:8787",
  settings: { productRootParent: "/data/relkit", productRootParentSource: "config" },
};

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function renderApp(initialPath: string, client = new QueryClient({
  defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
})) {
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[initialPath]}>
        <App />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.unstubAllGlobals();
  try {
    window.localStorage.clear();
  } catch {
    // The theme falls back to the system preference when storage is absent.
  }
});

describe("finding #2: product root default", () => {
  it("shows the parent directory the agent will really use, not a hardcoded path", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes("/api/session")) return jsonResponse(readySession);
        if (url.includes("/api/products")) return jsonResponse({ ok: true, products: [] });
        return jsonResponse({ ok: false, code: "not_found", message: "no such endpoint" }, 404);
      }),
    );

    const user = userEvent.setup();
    renderApp("/products");

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "注册产品" })).toBeInTheDocument();
    });
    await user.click(screen.getByRole("button", { name: "注册产品" }));

    // The dialog says where the product will land before anything is written.
    await waitFor(() => {
      expect(screen.getByText("/data/relkit/<产品名>")).toBeInTheDocument();
    });
    // The machine's real path, and never the old hardcoded /srv/relkit.
    expect(screen.queryByText(/\/srv\/relkit/)).not.toBeInTheDocument();
  });

  it("follows the product name once it is typed", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes("/api/session")) return jsonResponse(readySession);
        if (url.includes("/api/products")) return jsonResponse({ ok: true, products: [] });
        return jsonResponse({ ok: false, code: "not_found", message: "no such endpoint" }, 404);
      }),
    );

    const user = userEvent.setup();
    renderApp("/products");
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "注册产品" })).toBeInTheDocument();
    });
    await user.click(screen.getByRole("button", { name: "注册产品" }));
    await user.type(screen.getByLabelText(/产品名/), "loom");

    await waitFor(() => {
      expect(screen.getByText("/data/relkit/loom")).toBeInTheDocument();
    });
  });

  it("puts the agent's refusal under the root field", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (url.includes("/api/session")) return jsonResponse(readySession);
        if (url.includes("/api/products") && init?.method === "POST") {
          return jsonResponse(
            {
              ok: false,
              code: "product_root_unwritable",
              message: "无法创建目录 /data/relkit/demo：permission denied",
              field: "root",
            },
            400,
          );
        }
        if (url.includes("/api/products")) return jsonResponse({ ok: true, products: [] });
        return jsonResponse({ ok: false, code: "not_found", message: "no such endpoint" }, 404);
      }),
    );

    const user = userEvent.setup();
    renderApp("/products");
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "注册产品" })).toBeInTheDocument();
    });
    await user.click(screen.getByRole("button", { name: "注册产品" }));
    await user.type(screen.getByLabelText(/产品名/), "demo");
    await user.click(screen.getByRole("button", { name: "注册" }));

    await waitFor(() => {
      expect(screen.getByText(/无法创建目录 \/data\/relkit\/demo/)).toBeInTheDocument();
    });
  });
});

describe("finding #3: writes never replay on refresh", () => {
  it("sends exactly one rotate request when the confirm button is double clicked", async () => {
    let rotateCalls = 0;
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes("/api/session")) return jsonResponse(readySession);
      if (url.endsWith("/rotate")) {
        rotateCalls += 1;
        return jsonResponse({
          ok: true,
          token: {
            id: "tok-1",
            product: "loom",
            createdAt: "2026-10-01T00:00:00Z",
            rotatedAt: "2026-10-02T00:00:00Z",
          },
          plaintext: "relkit_live_once",
        });
      }
      if (url.includes("/tokens")) {
        return jsonResponse({
          ok: true,
          tokens: [
            { id: "tok-1", product: "loom", createdAt: "2026-10-01T00:00:00Z" },
          ],
        });
      }
      return jsonResponse({ ok: false, code: "not_found", message: "no such endpoint" }, 404);
    });
    vi.stubGlobal("fetch", fetchMock);

    const user = userEvent.setup();
    renderApp("/products/loom/tokens");

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "轮换" })).toBeInTheDocument();
    });
    await user.click(screen.getByRole("button", { name: "轮换" }));
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "轮换" })).toBeInTheDocument();
    });

    // The confirm button inside the dialog, not the row button.
    const confirm = screen.getAllByRole("button", { name: "轮换" }).at(-1)!;
    await user.click(confirm);
    await user.click(confirm);

    // The plaintext dialog appears once, and the request went out once: a
    // second click while the first is in flight must not rotate again.
    await waitFor(() => {
      expect(screen.getByText("relkit_live_once")).toBeInTheDocument();
    });
    expect(rotateCalls).toBe(1);
    expect(initPostCount(fetchMock, "/rotate")).toBe(1);
  });

  it("keeps the plaintext out of the token list", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes("/api/session")) return jsonResponse(readySession);
        if (url.includes("/tokens")) {
          return jsonResponse({
            ok: true,
            tokens: [
              { id: "tok-1", product: "loom", createdAt: "2026-10-01T00:00:00Z" },
            ],
          });
        }
        return jsonResponse({ ok: false, code: "not_found", message: "no such endpoint" }, 404);
      }),
    );

    renderApp("/products/loom/tokens");
    await waitFor(() => {
      expect(screen.getByText("tok-1")).toBeInTheDocument();
    });
    // A one-time secret must never be recoverable from the listing.
    expect(screen.queryByText(/relkit_live/)).not.toBeInTheDocument();
  });

  it("does not fire a write for a plain page load", async () => {
    const writes: string[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (init?.method && init.method !== "GET") writes.push(url);
        if (url.includes("/api/session")) return jsonResponse(readySession);
        if (url.includes("/tokens")) {
          return jsonResponse({ ok: true, tokens: [] });
        }
        return jsonResponse({ ok: false, code: "not_found", message: "no such endpoint" }, 404);
      }),
    );

    renderApp("/products/loom/tokens");
    await waitFor(() => {
      expect(screen.getByText("还没有令牌")).toBeInTheDocument();
    });
    expect(writes).toEqual([]);
  });
});

function initPostCount(
  fetchMock: ReturnType<typeof vi.fn>,
  fragment: string,
): number {
  return fetchMock.mock.calls.filter((call) => {
    const [input, init] = call as [RequestInfo | URL, RequestInit | undefined];
    return init?.method === "POST" && String(input).includes(fragment);
  }).length;
}

describe("keyboard: dialogs give focus back", () => {
  it("returns focus to the button that opened the dialog, so Tab continues from there", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes("/api/session")) return jsonResponse(readySession);
        if (url.includes("/api/products")) return jsonResponse({ ok: true, products: [] });
        return jsonResponse({ ok: false, code: "not_found", message: "no such endpoint" }, 404);
      }),
    );

    const user = userEvent.setup();
    renderApp("/products");
    const opener = await screen.findByRole("button", { name: "注册产品" });

    // Keyboard only: focus the opener, open with Enter, close with Escape.
    opener.focus();
    expect(opener).toHaveFocus();
    await user.keyboard("{Enter}");
    await waitFor(() => {
      expect(screen.getByRole("dialog", { name: "注册产品" })).toBeInTheDocument();
    });

    await user.keyboard("{Escape}");
    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });

    // Without the fix focus lands on <body> and the next Tab restarts at the
    // skip link, which reads as the keyboard losing its place.
    expect(opener).toHaveFocus();
  });
});
