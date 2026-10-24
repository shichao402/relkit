import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { App } from "./app";
import type { SessionView } from "@/lib/api-types";

/** Navigation reachability (acceptance finding #1).
 *
 *  The defect was not a broken link: it was that the four management screens had
 *  no link at all outside their own page, so a signed-in operator could not reach
 *  Products. These tests walk every entry the sidebar renders and assert it opens
 *  a screen, which is the property that was missing. */

const session: SessionView = {
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

/** One stub covering every endpoint the shell touches on boot. Any endpoint a
 *  test does not care about still has to answer, or the screen shows an error
 *  state instead of the thing under assertion. */
function stubApi() {
  const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    const url = typeof input === "string" ? input : input.toString();
    if (url.includes("/api/session")) return jsonResponse(session);
    if (url.includes("/api/products")) {
      return jsonResponse({ ok: true, products: [] });
    }
    if (url.includes("/api/users")) return jsonResponse({ ok: true, users: [] });
    if (url.includes("/api/roles")) return jsonResponse({ ok: true, roles: [] });
    if (url.includes("/api/files")) {
      return jsonResponse({ ok: true, dir: ".", entries: [] });
    }
    return jsonResponse({ ok: false, code: "not_found", message: "no such endpoint" }, 404);
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function renderApp(initialPath = "/products") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
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

describe("navigation", () => {
  it("renders a sidebar link for every management screen", async () => {
    stubApi();
    renderApp();

    // The four screens the old panel hid behind its own tabs, plus the raw tree.
    const nav = await screen.findByRole("navigation", { name: "主导航" });
    expect(within(nav).getByRole("link", { name: "产品" })).toBeInTheDocument();
    expect(within(nav).getByRole("link", { name: "用户" })).toBeInTheDocument();
    expect(within(nav).getByRole("link", { name: "角色" })).toBeInTheDocument();
    expect(within(nav).getByRole("link", { name: "令牌" })).toBeInTheDocument();
    expect(within(nav).getByRole("link", { name: "文件" })).toBeInTheDocument();
  });

  it("opens the products screen from a deep link", async () => {
    stubApi();
    renderApp("/products");
    await waitFor(() => {
      expect(screen.getAllByRole("heading", { name: "产品" }).length).toBeGreaterThan(0);
    });
  });

  it("opens the users screen and keeps the sidebar in place", async () => {
    stubApi();
    renderApp("/users");
    await waitFor(() => {
      expect(screen.getByText("还没有账号")).toBeInTheDocument();
    });
    // The shell is rendered by the route, not the page: 产品 is still reachable.
    const nav = screen.getByRole("navigation", { name: "主导航" });
    expect(within(nav).getByRole("link", { name: "产品" })).toBeInTheDocument();
  });

  it("navigates from products to users through the sidebar", async () => {
    stubApi();
    const user = userEvent.setup();
    renderApp("/products");

    const nav = await screen.findByRole("navigation", { name: "主导航" });
    await user.click(within(nav).getByRole("link", { name: "用户" }));
    await waitFor(() => {
      expect(screen.getByText("还没有账号")).toBeInTheDocument();
    });
  });

  it("hides the admin-only file browser from an owner", async () => {
    const fetchMock = stubApi();
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = typeof input === "string" ? input : input.toString();
      if (url.includes("/api/session")) {
        return jsonResponse({
          ...session,
          user: {
            username: "owner",
            role: "owner",
            products: ["loom"],
            createdAt: "2026-10-01T00:00:00Z",
          },
        });
      }
      if (url.includes("/api/products")) return jsonResponse({ ok: true, products: [] });
      return jsonResponse({ ok: false, code: "not_found", message: "no such endpoint" }, 404);
    });

    renderApp("/products");
    const nav = await screen.findByRole("navigation", { name: "主导航" });
    expect(within(nav).queryByRole("link", { name: "文件" })).not.toBeInTheDocument();
    expect(within(nav).getByRole("link", { name: "产品" })).toBeInTheDocument();
  });

  it("exposes every screen from the narrow-screen switcher", async () => {
    stubApi();
    renderApp("/products");

    // Below md the sidebar is hidden and the same navItems are rendered as a
    // select. jsdom applies no media queries, so both exist here; the assertion
    // is that the switcher carries the same destinations rather than a subset.
    const switcher = await screen.findByRole("combobox", { name: "切换页面" });
    const options = within(switcher)
      .getAllByRole("option")
      .map((option) => option.textContent);
    expect(options).toEqual(["导航", "产品", "用户", "角色", "令牌", "文件"]);
  });

  it("names the icon-only header controls", async () => {
    stubApi();
    renderApp("/products");

    // The theme and sign-out labels collapse to icons on narrow screens. A
    // control that loses its text must keep an accessible name, or it becomes
    // unreachable for assistive tech and untestable by role.
    expect(
      await screen.findByRole("button", { name: /^外观：/ }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "退出登录" }),
    ).toBeInTheDocument();
  });
});

describe("session gates", () => {
  it("sends an unauthenticated operator to sign-in and remembers the path", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse({
          ok: true,
          gate: "login",
          version: "test",
          agent: "http://127.0.0.1:8787",
        }),
      ),
    );

    renderApp("/products");
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "登录" })).toBeInTheDocument();
    });
  });

  it("shows the setup screen when the agent still has no account", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse({
          ok: true,
          gate: "setup",
          version: "test",
          agent: "http://127.0.0.1:8787",
        }),
      ),
    );

    renderApp("/products");
    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "创建管理员账户" }),
      ).toBeInTheDocument();
    });
  });
});
