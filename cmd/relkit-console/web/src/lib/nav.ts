import {
  FolderTreeIcon,
  KeyRoundIcon,
  PackageIcon,
  ShieldCheckIcon,
  UsersIcon,
  type LucideIcon,
} from "lucide-react";

/** The single source of truth for navigation (acceptance finding #1).
 *
 *  The old panel kept its four tabs inside one template, so nothing outside that
 *  page linked to them and a signed-in operator could not reach Products at all.
 *  Every destination now lives in this array: the sidebar renders it, the router
 *  mounts it, and the navigation test walks it and asserts each entry opens its
 *  page. A page that is not listed here does not exist as far as the shell is
 *  concerned, and a test fails if a listed route has no element.
 */

export interface NavItem {
  /** Route path, also the value matched for the active highlight. */
  to: string;
  label: string;
  description: string;
  icon: LucideIcon;
  /** Only rendered for administrators; the sidebar hides it from an owner and
   *  the API refuses it anyway. */
  adminOnly?: boolean;
}

export const navItems: NavItem[] = [
  {
    to: "/products",
    label: "产品",
    description: "已注册的产品、渠道与发布包",
    icon: PackageIcon,
  },
  {
    to: "/users",
    label: "用户",
    description: "账户、角色与可管理的产品",
    icon: UsersIcon,
  },
  {
    to: "/roles",
    label: "角色",
    description: "管理员与产品负责人的权限边界",
    icon: ShieldCheckIcon,
  },
  {
    to: "/tokens",
    label: "令牌",
    description: "发布令牌的签发、轮换与吊销",
    icon: KeyRoundIcon,
  },
  {
    to: "/files",
    label: "文件",
    description: "发布树原始键空间（只读）",
    icon: FolderTreeIcon,
    adminOnly: true,
  },
];

/** Deep pages reached from a list row, not from the sidebar. They are listed so
 *  the navigation test covers them too, and so breadcrumbs can name them. */
export const detailRoutes = {
  product: (id: string) => `/products/${encodeURIComponent(id)}`,
  productTokens: (id: string) => `/products/${encodeURIComponent(id)}/tokens`,
};

export function activeNavItem(pathname: string): NavItem | undefined {
  // Longest prefix wins so /products/foo highlights 产品 and not nothing.
  return [...navItems]
    .sort((a, b) => b.to.length - a.to.length)
    .find((item) => pathname === item.to || pathname.startsWith(item.to + "/"));
}
