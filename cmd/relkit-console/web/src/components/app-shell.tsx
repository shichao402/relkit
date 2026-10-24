import * as React from "react";
import { NavLink, useLocation, useNavigate } from "react-router-dom";
import { LogOutIcon, MonitorIcon, MoonIcon, SunIcon } from "lucide-react";
import { navItems, activeNavItem } from "@/lib/nav";
import { useSession } from "@/lib/session";
import { useTheme, type ThemeChoice } from "@/lib/theme";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

/** The shell every signed-in screen sits in.
 *
 *  Acceptance finding #1 was not a missing page, it was a missing way to reach
 *  one: the management tabs existed only inside the page they belonged to. The
 *  sidebar is therefore rendered by the shell, from navItems, on every route,
 *  and never by a page itself. */

export function AppShell({ children }: { children: React.ReactNode }) {
  const { session, signOut } = useSession();
  const navigate = useNavigate();
  const isAdmin = session?.user?.role === "admin";
  const items = navItems.filter((item) => !item.adminOnly || isAdmin);

  return (
    <div className="bg-background flex min-h-screen">
      <a
        href="#main"
        className="bg-card focus:ring-ring sr-only rounded-md px-3 py-2 text-sm focus:not-sr-only focus:absolute focus:top-2 focus:left-2 focus:z-50 focus:ring-2"
      >
        跳到主内容
      </a>

      <aside className="bg-card hidden w-60 shrink-0 flex-col border-r md:flex">
        <div className="flex h-14 items-center gap-2 border-b px-4">
          <span className="text-sm font-semibold tracking-tight">relkit 控制台</span>
        </div>
        <nav aria-label="主导航" className="flex-1 space-y-1 p-3">
          {items.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              className={({ isActive }) =>
                cn(
                  "flex items-start gap-2.5 rounded-md px-3 py-2 text-sm transition-colors",
                  isActive
                    ? "bg-primary/10 text-primary font-medium"
                    : "text-muted-foreground hover:bg-accent hover:text-accent-foreground",
                )
              }
            >
              <item.icon className="mt-0.5 size-4 shrink-0" />
              <span className="min-w-0">
                <span className="block">{item.label}</span>
              </span>
            </NavLink>
          ))}
        </nav>
        <div className="text-muted-foreground space-y-1 border-t px-4 py-3 text-xs">
          <p className="truncate">版本 {session?.version ?? "—"}</p>
          {session?.settings?.productRootParent && (
            <p className="truncate" title={session.settings.productRootParent}>
              产品根父目录 {session.settings.productRootParent}
            </p>
          )}
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="bg-card/85 sticky top-0 z-30 flex h-14 items-center gap-3 border-b px-4 backdrop-blur">
          <MobileNav items={items} />
          <Breadcrumbs />
          <div className="ml-auto flex items-center gap-2">
            <ThemeMenu />
            <div className="hidden items-center gap-2 sm:flex">
              <span className="text-sm">{session?.user?.username}</span>
              <Badge variant={isAdmin ? "primary" : "neutral"}>
                {isAdmin ? "管理员" : "产品负责人"}
              </Badge>
            </div>
            <Button
              variant="ghost"
              size="sm"
              aria-label="退出登录"
              onClick={async () => {
                await signOut();
                navigate("/sign-in", { replace: true });
              }}
            >
              <LogOutIcon />
              <span className="hidden sm:inline">退出</span>
            </Button>
          </div>
        </header>

        <main id="main" className="mx-auto w-full max-w-[88rem] flex-1 px-4 py-6 sm:px-6">
          {children}
        </main>
      </div>
    </div>
  );
}

/** Narrow screens lose the sidebar, so the same items are reachable from a
 *  select in the header. It is not a drawer with its own state: one control,
 *  the same navItems, no second definition of the menu to drift. */
function MobileNav({ items }: { items: typeof navItems }) {
  const navigate = useNavigate();
  const { pathname } = useLocation();
  const active = activeNavItem(pathname);
  return (
    <div className="md:hidden">
      <label className="sr-only" htmlFor="mobile-nav">
        切换页面
      </label>
      <select
        id="mobile-nav"
        className="border-input bg-background h-8 rounded-md border px-2 text-sm"
        value={active?.to ?? ""}
        onChange={(event) => {
          if (event.target.value) navigate(event.target.value);
        }}
      >
        <option value="" disabled>
          导航
        </option>
        {items.map((item) => (
          <option key={item.to} value={item.to}>
            {item.label}
          </option>
        ))}
      </select>
    </div>
  );
}

function Breadcrumbs() {
  const { pathname } = useLocation();
  const active = activeNavItem(pathname);
  if (!active) return <span className="text-sm font-medium">relkit 控制台</span>;
  const rest = pathname.slice(active.to.length).split("/").filter(Boolean);
  return (
    <nav aria-label="面包屑" className="flex min-w-0 items-center gap-1.5 text-sm">
      <NavLink to={active.to} className="font-medium hover:underline">
        {active.label}
      </NavLink>
      {rest.map((segment, index) => (
        <React.Fragment key={index}>
          <span className="text-muted-foreground/60">/</span>
          <span className="text-muted-foreground truncate">{decodeURIComponent(segment)}</span>
        </React.Fragment>
      ))}
    </nav>
  );
}

const themeOptions: { value: ThemeChoice; label: string; icon: typeof SunIcon }[] = [
  { value: "light", label: "浅色", icon: SunIcon },
  { value: "dark", label: "深色", icon: MoonIcon },
  { value: "system", label: "跟随系统", icon: MonitorIcon },
];

/** A three-state control instead of a light/dark switch: "system" is a real
 *  choice, and a plain toggle cannot express it. */
function ThemeMenu() {
  const { choice, setChoice } = useTheme();
  const current = themeOptions.find((option) => option.value === choice) ?? themeOptions[2];
  const [open, setOpen] = React.useState(false);
  const ref = React.useRef<HTMLDivElement>(null);

  React.useEffect(() => {
    if (!open) return;
    const onPointer = (event: MouseEvent) => {
      if (!ref.current?.contains(event.target as Node)) setOpen(false);
    };
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") setOpen(false);
    };
    document.addEventListener("mousedown", onPointer);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onPointer);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  return (
    <div className="relative" ref={ref}>
      <Button
        variant="ghost"
        size="sm"
        aria-haspopup="menu"
        aria-expanded={open}
        // The label collapses to an icon below lg, so the button needs a name of
        // its own; otherwise it is an unnamed control for screen readers and for
        // the keyboard/accessibility checks.
        aria-label={`外观：${current.label}`}
        onClick={() => setOpen((value) => !value)}
      >
        <current.icon />
        <span className="hidden lg:inline">{current.label}</span>
      </Button>
      {open && (
        <div
          role="menu"
          className="bg-popover text-popover-foreground absolute right-0 z-50 mt-1 w-36 overflow-hidden rounded-md border shadow-lg"
        >
          {themeOptions.map((option) => (
            <button
              key={option.value}
              role="menuitemradio"
              aria-checked={choice === option.value}
              className={cn(
                "hover:bg-accent flex w-full items-center gap-2 px-3 py-2 text-left text-sm",
                choice === option.value && "text-primary font-medium",
              )}
              onClick={() => {
                setChoice(option.value);
                setOpen(false);
              }}
            >
              <option.icon className="size-4" />
              {option.label}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

/** Page heading used by every screen, so titles and actions line up. */
export function PageHeader({
  title,
  description,
  actions,
}: {
  title: string;
  description?: React.ReactNode;
  actions?: React.ReactNode;
}) {
  return (
    <div className="mb-5 flex flex-wrap items-start justify-between gap-3">
      <div className="min-w-0">
        <h1 className="text-xl font-semibold tracking-tight">{title}</h1>
        {description && (
          <p className="text-muted-foreground mt-1 text-sm">{description}</p>
        )}
      </div>
      {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
    </div>
  );
}
