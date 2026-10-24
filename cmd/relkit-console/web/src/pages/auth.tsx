import * as React from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { useMutation } from "@tanstack/react-query";
import { api, ApiRequestError } from "@/lib/api";
import { useSession } from "@/lib/session";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

/** Sign-in and first-run setup.
 *
 *  Both replace the old server-rendered forms. The difference that matters is
 *  that these are plain JSON calls: a failed attempt shows the server's message
 *  next to the field it names, and there is no form POST for a browser to
 *  replay. */

interface AuthCardProps {
  title: string;
  description: React.ReactNode;
  children: React.ReactNode;
  footer?: React.ReactNode;
}

function AuthLayout({ title, description, children, footer }: AuthCardProps) {
  return (
    <div className="bg-background flex min-h-screen items-center justify-center px-4 py-10">
      <div className="w-full max-w-sm">
        <div className="mb-6 text-center">
          <h1 className="text-lg font-semibold tracking-tight">relkit 控制台</h1>
        </div>
        <Card>
          <CardHeader className="gap-1">
            <CardTitle>{title}</CardTitle>
            <CardDescription>{description}</CardDescription>
          </CardHeader>
          <CardContent>{children}</CardContent>
        </Card>
        {footer && (
          <p className="text-muted-foreground mt-4 text-center text-xs">{footer}</p>
        )}
      </div>
    </div>
  );
}

/** Field errors come from the API envelope: `field` names the input, so the
 *  message lands where the operator is looking instead of in a banner. */
function useFieldError() {
  const [error, setError] = React.useState<ApiRequestError | null>(null);
  const messageFor = (field: string) =>
    error?.field === field ? error.message : undefined;
  const general = error && !error.field ? error.message : undefined;
  return { error, setError, messageFor, general };
}

export function SignInPage() {
  const [username, setUsername] = React.useState("");
  const [password, setPassword] = React.useState("");
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const { refresh } = useSession();
  const fields = useFieldError();

  const login = useMutation({
    mutationFn: () => api.login(username, password),
    onSuccess: async () => {
      await refresh();
      navigate(params.get("next") || "/products", { replace: true });
    },
    onError: (error) => fields.setError(error as ApiRequestError),
  });

  return (
    <AuthLayout title="登录" description="使用 agent 上的操作员账户登录。">
      <form
        className="space-y-3"
        onSubmit={(event) => {
          event.preventDefault();
          fields.setError(null);
          login.mutate();
        }}
      >
        <div className="space-y-1.5">
          <Label htmlFor="username">用户名</Label>
          <Input
            id="username"
            autoComplete="username"
            autoFocus
            value={username}
            onChange={(event) => setUsername(event.target.value)}
            aria-invalid={Boolean(fields.messageFor("username"))}
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="password">密码</Label>
          <Input
            id="password"
            type="password"
            autoComplete="current-password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            aria-invalid={Boolean(fields.messageFor("password"))}
          />
        </div>
        {fields.general && (
          <p className="text-destructive text-sm" role="alert">
            {fields.general}
          </p>
        )}
        <Button type="submit" className="w-full" disabled={login.isPending}>
          {login.isPending ? "登录中…" : "登录"}
        </Button>
      </form>
    </AuthLayout>
  );
}

export function SetupPage() {
  const [bootstrap, setBootstrap] = React.useState("");
  const [username, setUsername] = React.useState("");
  const [password, setPassword] = React.useState("");
  const [confirm, setConfirm] = React.useState("");
  const [localError, setLocalError] = React.useState<string | null>(null);
  const navigate = useNavigate();
  const { refresh } = useSession();
  const fields = useFieldError();

  const setup = useMutation({
    mutationFn: () => api.setup(bootstrap, username, password),
    onSuccess: async () => {
      await refresh();
      navigate("/products", { replace: true });
    },
    onError: (error) => fields.setError(error as ApiRequestError),
  });

  return (
    <AuthLayout
      title="首次设置"
      description="输入 agent 启动时打印的一次性凭据，创建第一个管理员账户。"
      footer="凭据只能使用一次；创建账户后由你自己保管用户名与密码。"
    >
      <form
        className="space-y-3"
        onSubmit={(event) => {
          event.preventDefault();
          fields.setError(null);
          setLocalError(null);
          if (password.length < 8) {
            setLocalError("密码至少 8 位");
            return;
          }
          if (password !== confirm) {
            setLocalError("两次输入的密码不一致");
            return;
          }
          setup.mutate();
        }}
      >
        <div className="space-y-1.5">
          <Label htmlFor="bootstrap">一次性凭据</Label>
          <Input
            id="bootstrap"
            autoFocus
            autoComplete="off"
            className="font-mono"
            value={bootstrap}
            onChange={(event) => setBootstrap(event.target.value)}
            aria-invalid={Boolean(fields.messageFor("bootstrap"))}
          />
          {fields.messageFor("bootstrap") && (
            <p className="text-destructive text-xs" role="alert">
              {fields.messageFor("bootstrap")}
            </p>
          )}
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="setup-username">用户名</Label>
          <Input
            id="setup-username"
            autoComplete="username"
            value={username}
            onChange={(event) => setUsername(event.target.value)}
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="setup-password">密码</Label>
          <Input
            id="setup-password"
            type="password"
            autoComplete="new-password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            aria-invalid={Boolean(fields.messageFor("password"))}
          />
          {fields.messageFor("password") && (
            <p className="text-destructive text-xs" role="alert">
              {fields.messageFor("password")}
            </p>
          )}
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="setup-confirm">确认密码</Label>
          <Input
            id="setup-confirm"
            type="password"
            autoComplete="new-password"
            value={confirm}
            onChange={(event) => setConfirm(event.target.value)}
          />
        </div>
        {(localError || fields.general) && (
          <p className="text-destructive text-sm" role="alert">
            {localError ?? fields.general}
          </p>
        )}
        <Button type="submit" className="w-full" disabled={setup.isPending}>
          {setup.isPending ? "创建中…" : "创建管理员账户"}
        </Button>
      </form>
    </AuthLayout>
  );
}

/** Two states that are not the operator's fault and have no form: the panel is
 *  locked or the agent cannot be reached. Saying which one it is is the whole
 *  point, because the fix differs. */
export function BlockedPage({
  kind,
  detail,
}: {
  kind: "locked" | "unavailable";
  detail?: string;
}) {
  return (
    <AuthLayout
      title={kind === "locked" ? "面板已锁定" : "agent 不可达"}
      description={
        kind === "locked"
          ? "控制台没有可用的 agent 配置，管理功能已停用。"
          : "控制台无法连接发布机上的 agent，管理功能暂时不可用。"
      }
    >
      <div className="text-muted-foreground space-y-2 text-sm">
        <p>请联系维护者处理：</p>
        <ul className="list-disc space-y-1 pl-5">
          {kind === "locked" ? (
            <li>在 relkit-console 配置中补上 agent 地址后重启服务。</li>
          ) : (
            <li>确认内网发布机上的 relkit-agent 正在运行。</li>
          )}
        </ul>
        {detail && <p className="text-xs break-all">{detail}</p>}
      </div>
    </AuthLayout>
  );
}
