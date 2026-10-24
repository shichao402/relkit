import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { App } from "./app";
import type { SessionView } from "@/lib/api-types";
import { translateApiMessage } from "@/lib/api";

/** Acceptance findings found on the machine, round two.
 *
 *  Each test names the defect it guards. These are the failures that only show
 *  up when the panel is actually driven, not when a single endpoint is called:
 *  leaving the panel, creating an owner, and reading a refusal that came back
 *  from the agent in English. */

const signedIn: SessionView = {
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

function renderApp(initialPath: string) {
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

describe("finding #4: signing out lands on the sign-in screen", () => {
  it("shows the sign-in form right after the sign-out button is pressed", async () => {
    // The session endpoint keeps answering "ready" for the first call (the
    // boot), and answers "login" only after the logout completes, which is what
    // the real server does: the cookie is gone from that request onward.
    let loggedOut = false;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes("/api/session/logout")) {
          loggedOut = true;
          return jsonResponse({ ok: true });
        }
        if (url.includes("/api/session")) {
          if (loggedOut) {
            return jsonResponse({
              ok: true,
              gate: "login",
              version: "test",
              agent: "http://127.0.0.1:8787",
            });
          }
          return jsonResponse(signedIn);
        }
        if (url.includes("/api/products")) {
          return jsonResponse({ ok: true, products: [] });
        }
        return jsonResponse({ ok: false, code: "not_found", message: "no such API endpoint" }, 404);
      }),
    );

    const user = userEvent.setup();
    renderApp("/products");

    await user.click(await screen.findByRole("button", { name: "退出登录" }));

    // Before the fix the panel stayed on screen: the cache was cleared, the
    // session query went pending, and the panel's own view was never replaced.
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "登录" })).toBeInTheDocument();
    });
    expect(screen.queryByRole("button", { name: "退出登录" })).not.toBeInTheDocument();
    // And no stale English refusal is left behind on top of the page.
    expect(screen.queryByText("sign in to continue")).not.toBeInTheDocument();
  });
});

describe("finding #5: an owner needs a product at creation time", () => {
  it("offers the product list and sends the chosen products", async () => {
    const bodies: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (url.includes("/api/session")) return jsonResponse(signedIn);
        if (url.includes("/api/users") && init?.method === "POST") {
          bodies.push(JSON.parse(String(init.body)));
          return jsonResponse({
            ok: true,
            user: {
              username: "owner",
              role: "owner",
              products: ["loom"],
              createdAt: "2026-10-02T00:00:00Z",
            },
          });
        }
        if (url.includes("/api/users")) return jsonResponse({ ok: true, users: [] });
        if (url.includes("/api/products")) {
          return jsonResponse({
            ok: true,
            products: [
              { id: "loom", display: "Loom", registered: true, published: false, downloads: 0, channels: [] },
            ],
          });
        }
        return jsonResponse({ ok: false, code: "not_found", message: "no such API endpoint" }, 404);
      }),
    );

    const user = userEvent.setup();
    renderApp("/users");
    await user.click(await screen.findByRole("button", { name: "新建账号" }));

    await user.type(screen.getByLabelText("用户名"), "owner");
    await user.type(screen.getByLabelText("初始密码"), "secret-pass");

    // An owner with nothing checked cannot be submitted: that combination is
    // exactly what the agent refuses.
    const create = screen.getByRole("button", { name: "创建" });
    expect(create).toBeDisabled();

    await user.click(await screen.findByRole("checkbox", { name: "loom" }));
    await waitFor(() => {
      expect(create).toBeEnabled();
    });
    await user.click(create);

    await waitFor(() => {
      expect(bodies).toHaveLength(1);
    });
    expect(bodies[0]).toMatchObject({ username: "owner", role: "owner", products: ["loom"] });
  });

  it("does not ask for products when the new account is an administrator", async () => {
    const bodies: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (url.includes("/api/session")) return jsonResponse(signedIn);
        if (url.includes("/api/users") && init?.method === "POST") {
          bodies.push(JSON.parse(String(init.body)));
          return jsonResponse({
            ok: true,
            user: { username: "boss", role: "admin", createdAt: "2026-10-02T00:00:00Z" },
          });
        }
        if (url.includes("/api/users")) return jsonResponse({ ok: true, users: [] });
        if (url.includes("/api/products")) {
          return jsonResponse({ ok: true, products: [] });
        }
        return jsonResponse({ ok: false, code: "not_found", message: "no such API endpoint" }, 404);
      }),
    );

    const user = userEvent.setup();
    renderApp("/users");
    await user.click(await screen.findByRole("button", { name: "新建账号" }));
    await user.type(screen.getByLabelText("用户名"), "boss");
    await user.type(screen.getByLabelText("初始密码"), "secret-pass");
    await user.selectOptions(screen.getByLabelText("角色"), "admin");

    const create = screen.getByRole("button", { name: "创建" });
    await waitFor(() => {
      expect(create).toBeEnabled();
    });
    await user.click(create);
    await waitFor(() => {
      expect(bodies).toHaveLength(1);
    });
    expect(bodies[0]).toMatchObject({ username: "boss", role: "admin", products: [] });
  });
});

describe("refusals are shown in the panel's language", () => {
  it("translates the fixed sentences the agent and panel answer with", () => {
    expect(translateApiMessage("a product owner needs at least one product")).toBe(
      "产品负责人至少要授权一个产品",
    );
    expect(translateApiMessage("sign in to continue")).toBe("登录已失效，请重新登录");
    expect(translateApiMessage("no such product")).toBe("产品不存在");
  });

  it("leaves a message with a computed tail alone instead of half-translating it", () => {
    const original = "invalid JSON: unexpected end of JSON input";
    expect(translateApiMessage(original)).toBe(original);
  });
});
