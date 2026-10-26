import type {
  ChannelDetail,
  FileEntry,
  IssuedToken,
  PanelSettings,
  ProductDetail,
  ProductSummary,
  RetireResult,
  RoleView,
  SessionUser,
  SessionView,
  Token,
} from "./api-types";

/** The panel API lives under the app's own base path. */
const API = "/-/admin/api";

/** Thrown for every non-2xx answer so callers can show `message` verbatim and
 *  put `field` next to the input it belongs to. */
export class ApiRequestError extends Error {
  readonly status: number;
  readonly code: string;
  readonly field?: string;

  constructor(status: number, code: string, message: string, field?: string) {
    super(message);
    this.name = "ApiRequestError";
    this.status = status;
    this.code = code;
    this.field = field;
  }

  /** A session the agent no longer honours. The app sends the user to sign-in. */
  get unauthenticated(): boolean {
    return this.status === 401;
  }
}

/** Refusals that come back as a fixed English sentence: the panel answers in
 *  Chinese, so they are translated here once instead of in every screen. Only
 *  whole messages are listed; a message with a computed tail (a path, an OS
 *  error) is left alone because half-translating it would be worse. */
const messageTranslations: Record<string, string> = {
  "sign in to continue": "登录已失效，请重新登录",
  "a product owner needs at least one product": "产品负责人至少要授权一个产品",
  "only an administrator can browse the file tree": "只有管理员可以浏览文件树",
  "missing or stale CSRF token; reload the page": "请求已过期，请重新加载页面",
  "the panel has no agent configured": "控制台没有配置 agent",
  "no such API endpoint": "接口不存在",
  "no such product": "产品不存在",
  "no such directory": "目录不存在",
  "no such user action": "用户操作不存在",
  "no such token action": "令牌操作不存在",
  "method not allowed": "不支持该请求方法",
  "invalid directory": "目录参数无效",
  "request body is too large": "请求体过大",
  "could not read the request body": "无法读取请求体",
};

/** Turns a refusal into the sentence the panel shows. Unknown messages pass
 *  through verbatim: hiding the reason a call failed is worse than English. */
export function translateApiMessage(message: string): string {
  return messageTranslations[message.trim()] ?? message;
}

/** The CSRF token changes with every session, so it is held here and set by the
 *  session query rather than read from a cookie (the browser cannot read the
 *  signed one, and it must not be in a place a cross-site page can guess). */
let csrfToken: string | null = null;

export function setCsrfToken(token: string | null) {
  csrfToken = token;
}

interface RequestOptions {
  method?: string;
  body?: unknown;
  signal?: AbortSignal;
}

async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const method = options.method ?? "GET";
  const headers: Record<string, string> = { Accept: "application/json" };
  if (options.body !== undefined) headers["Content-Type"] = "application/json";
  if (csrfToken && method !== "GET" && method !== "HEAD") {
    headers["X-Relkit-CSRF"] = csrfToken;
  }
  const response = await fetch(API + path, {
    method,
    headers,
    credentials: "same-origin",
    signal: options.signal,
    body: options.body === undefined ? undefined : JSON.stringify(options.body),
  });

  const text = await response.text();
  let payload: unknown = null;
  if (text) {
    try {
      payload = JSON.parse(text);
    } catch {
      payload = null;
    }
  }

  if (!response.ok) {
    const envelope = (payload ?? {}) as {
      code?: string;
      message?: string;
      field?: string;
    };
    throw new ApiRequestError(
      response.status,
      envelope.code ?? "http_" + response.status,
      translateApiMessage(
        envelope.message ?? (text || `请求失败（HTTP ${response.status}）`),
      ),
      envelope.field,
    );
  }
  return payload as T;
}

export const api = {
  session: (signal?: AbortSignal) =>
    request<SessionView>("/session", { signal }),

  login: (username: string, password: string) =>
    request<SessionView>("/session/login", {
      method: "POST",
      body: { username, password },
    }),

  setup: (bootstrap: string, username: string, password: string) =>
    request<SessionView>("/session/setup", {
      method: "POST",
      body: { bootstrap, username, password },
    }),

  logout: () => request<{ ok: true }>("/session/logout", { method: "POST" }),

  settings: (signal?: AbortSignal) =>
    request<{ ok: true; settings: PanelSettings }>("/settings", { signal }),

  products: (signal?: AbortSignal) =>
    request<{ ok: true; products: ProductSummary[] }>("/products", { signal }),

  product: (id: string, signal?: AbortSignal) =>
    request<{ ok: true; product: ProductDetail }>(
      `/products/${encodeURIComponent(id)}`,
      { signal },
    ),

  addProduct: (input: {
    id: string;
    root: string;
    profile: string;
    label: string;
  }) => request<IssuedToken>("/products", { method: "POST", body: input }),

  // dryRun previews the retirement without touching anything: the agent looks at
  // what is actually published and reports what removal would take away.
  deleteProduct: (id: string, options: { dryRun?: boolean } = {}) =>
    request<RetireResult>(
      `/products/${encodeURIComponent(id)}${options.dryRun ? "?dryRun=1" : ""}`,
      { method: "DELETE" },
    ),

  tokens: (product: string, signal?: AbortSignal) =>
    request<{ ok: true; tokens: Token[] }>(
      `/products/${encodeURIComponent(product)}/tokens`,
      { signal },
    ),

  issueToken: (product: string, label: string) =>
    request<IssuedToken>(`/products/${encodeURIComponent(product)}/tokens`, {
      method: "POST",
      body: { label },
    }),

  rotateToken: (id: string) =>
    request<IssuedToken>(`/tokens/${encodeURIComponent(id)}/rotate`, {
      method: "POST",
    }),

  revokeToken: (id: string) =>
    request<IssuedToken>(`/tokens/${encodeURIComponent(id)}/revoke`, {
      method: "POST",
    }),

  users: (signal?: AbortSignal) =>
    request<{ ok: true; users: SessionUser[] }>("/users", { signal }),

  addUser: (input: {
    username: string;
    password: string;
    role: string;
    products: string[];
  }) => request<{ ok: true; user: SessionUser }>("/users", { method: "POST", body: input }),

  deleteUser: (name: string) =>
    request<{ ok: true }>(`/users/${encodeURIComponent(name)}`, {
      method: "DELETE",
    }),

  setPassword: (name: string, password: string) =>
    request<{ ok: true }>(`/users/${encodeURIComponent(name)}/password`, {
      method: "POST",
      body: { password },
    }),

  setRole: (name: string, role: string, products: string[]) =>
    request<{ ok: true; user: SessionUser }>(
      `/users/${encodeURIComponent(name)}/role`,
      { method: "POST", body: { role, products } },
    ),

  setUserProducts: (name: string, products: string[]) =>
    request<{ ok: true; user: SessionUser }>(
      `/users/${encodeURIComponent(name)}/products`,
      { method: "PUT", body: { products } },
    ),

  roles: (signal?: AbortSignal) =>
    request<{ ok: true; roles: RoleView[] }>("/roles", { signal }),

  files: (dir: string, signal?: AbortSignal) =>
    request<{ ok: true; dir: string; entries: FileEntry[] }>(
      `/files?dir=${encodeURIComponent(dir)}`,
      { signal },
    ),
};

export type { ChannelDetail, FileEntry, IssuedToken, ProductDetail, ProductSummary, Token };
