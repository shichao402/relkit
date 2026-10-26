/** Mirrors of the console JSON API (`cmd/relkit-console/api*.go`). Field names
 *  are the wire names: the Go side owns them and these must not drift. */

export type Gate = "ready" | "login" | "setup" | "locked" | "unavailable";

export interface ApiError {
  ok: false;
  code: string;
  message: string;
  field?: string;
}

export interface SessionUser {
  username: string;
  role: string;
  products?: string[];
  createdAt: string;
}

export interface PanelSettings {
  productRootParent: string;
  productRootParentSource: string;
}

export interface SessionView {
  ok: true;
  gate: Gate;
  user?: SessionUser;
  csrf?: string;
  version: string;
  agent: string;
  settings?: PanelSettings;
}

export interface ChannelSummary {
  name: string;
  version?: string;
  code?: number;
  released?: string;
  yanked?: boolean;
  error?: string;
}

export interface ProductSummary {
  id: string;
  display: string;
  description?: string;
  homepage?: string;
  registered: boolean;
  published: boolean;
  root?: string;
  profile?: string;
  createdAt?: string;
  updated?: string;
  downloads: number;
  channels: ChannelSummary[];
  connectConfig?: Record<string, unknown>;
}

export interface Artifact {
  id: string;
  filename: string;
  platform: string;
  os?: string;
  arch?: string;
  size: string;
  sha256: string;
  href?: string;
  permanentHref?: string;
  downloads: number;
}

export interface Release {
  version: string;
  code: number;
  released?: string;
  yanked?: boolean;
  manifestHref?: string;
  notesUrl?: string;
}

export interface ChannelDetail {
  name: string;
  sequence: number;
  generated?: string;
  indexHref?: string;
  latestVersion?: string;
  latestCode?: number;
  latestReleased?: string;
  latestNotes?: string;
  artifacts: Artifact[];
  releases: Release[];
  truncated?: number;
  error?: string;
}

export interface ProductDetail extends ProductSummary {
  statsSince?: string;
  detail: ChannelDetail[];
}

export interface Token {
  id: string;
  product: string;
  label?: string;
  createdAt: string;
  rotatedAt?: string;
  revokedAt?: string;
}

export interface FileEntry {
  name: string;
  dir: boolean;
  size: number;
  mtime?: string;
}

export interface RoleView {
  name: string;
  members: number;
}

/** One-time credential: present only in the response that issued it. */
export interface IssuedToken {
  ok: true;
  token: Token;
  plaintext?: string;
}

/** What a retirement did, or would do when `dryRun` is set. The agent answers
 *  both from the same code path so a preview cannot promise something the real
 *  run will not do. */
export interface RetireResult {
  ok: true;
  dryRun?: boolean;
  removedTokens?: number;
  log?: string[];
  error?: string;
}
