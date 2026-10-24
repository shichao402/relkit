import * as React from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, ApiRequestError } from "@/lib/api";
import type { SessionUser } from "@/lib/api-types";
import { useSession } from "@/lib/session";
import { PageHeader } from "@/components/app-shell";
import { EmptyState, ErrorState } from "@/components/states";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { formatStamp } from "@/lib/utils";
import { PlusIcon, UsersIcon } from "lucide-react";

const roleLabels: Record<string, string> = {
  admin: "管理员",
  owner: "产品负责人",
};

function roleLabel(role: string) {
  return roleLabels[role] ?? role;
}

export function UsersPage() {
  const { session } = useSession();
  const isAdmin = session?.user?.role === "admin";
  const client = useQueryClient();
  const [creating, setCreating] = React.useState(false);
  const [editing, setEditing] = React.useState<SessionUser | null>(null);
  const [deleting, setDeleting] = React.useState<SessionUser | null>(null);

  const users = useQuery({
    queryKey: ["users"],
    queryFn: ({ signal }) => api.users(signal),
  });

  const invalidate = () => client.invalidateQueries({ queryKey: ["users"] });

  const remove = useMutation({
    mutationFn: (name: string) => api.deleteUser(name),
    onSuccess: async () => {
      setDeleting(null);
      await invalidate();
      toast.success("账号已删除");
    },
    onError: (error) => {
      setDeleting(null);
      toast.error((error as ApiRequestError).message);
    },
  });

  if (users.isPending) {
    return (
      <>
        <PageHeader title="用户" />
        <div className="space-y-2">
          {[0, 1, 2].map((index) => (
            <Skeleton key={index} className="h-14 w-full" />
          ))}
        </div>
      </>
    );
  }
  if (users.error) {
    return (
      <>
        <PageHeader title="用户" />
        <ErrorState
          message={(users.error as Error).message}
          onRetry={() => void users.refetch()}
        />
      </>
    );
  }

  const rows = users.data.users;

  return (
    <>
      <PageHeader
        title="用户"
        description="账号存放在 agent 上。管理员可以管理全部产品，产品负责人只能管理被授权的产品。"
        actions={
          <Button onClick={() => setCreating(true)} disabled={!isAdmin}>
            <PlusIcon />
            新建账号
          </Button>
        }
      />

      {rows.length === 0 ? (
        <EmptyState
          icon={<UsersIcon className="size-7" />}
          title="还没有账号"
          description="创建第一个账号，把发布权限交给对应的人或 CI。"
        />
      ) : (
        <div className="grid gap-2">
          {rows.map((user) => {
            const self = user.username === session?.user?.username;
            return (
              <Card key={user.username} className="gap-2 py-3">
                <div className="flex flex-wrap items-center gap-2 px-5">
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="text-sm font-medium">{user.username}</span>
                      {self && <Badge variant="outline">当前登录</Badge>}
                      <Badge variant={user.role === "admin" ? "primary" : "neutral"}>
                        {roleLabel(user.role)}
                      </Badge>
                    </div>
                    <div className="text-muted-foreground mt-1 flex flex-wrap gap-x-4 gap-y-1 text-xs">
                      <span>创建于 {formatStamp(user.createdAt)}</span>
                      {user.products && user.products.length > 0 ? (
                        <span>
                          可管理{" "}
                          {user.products.map((product) => (
                            <span
                              key={product}
                              className="bg-muted mr-1 rounded px-1 py-0.5 font-mono"
                            >
                              {product}
                            </span>
                          ))}
                        </span>
                      ) : (
                        user.role !== "admin" && <span>未授权任何产品</span>
                      )}
                    </div>
                  </div>
                  {isAdmin && (
                    <div className="flex gap-1.5">
                      <Button variant="outline" size="sm" onClick={() => setEditing(user)}>
                        编辑
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="text-destructive hover:bg-destructive/10"
                        disabled={self}
                        title={self ? "不能删除自己当前使用的账号" : undefined}
                        onClick={() => setDeleting(user)}
                      >
                        删除
                      </Button>
                    </div>
                  )}
                </div>
              </Card>
            );
          })}
        </div>
      )}

      <CreateUserDialog
        open={creating}
        onOpenChange={setCreating}
        onCreated={() => void invalidate()}
      />
      <EditUserDialog
        user={editing}
        onOpenChange={(open) => !open && setEditing(null)}
        onSaved={() => void invalidate()}
      />
      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(open) => !open && setDeleting(null)}
        title={`删除账号 ${deleting?.username ?? ""}`}
        description={
          <div className="space-y-1.5">
            <p>这个账号会立刻无法登录，它名下的令牌不受影响。</p>
            <p>删除后无法恢复，需要重新创建并分配产品权限。</p>
          </div>
        }
        confirmLabel="删除"
        pending={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting.username)}
      />
    </>
  );
}

function CreateUserDialog({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated: () => void;
}) {
  const [username, setUsername] = React.useState("");
  const [password, setPassword] = React.useState("");
  const [role, setRole] = React.useState("owner");
  const [error, setError] = React.useState<ApiRequestError | null>(null);

  const create = useMutation({
    mutationFn: () =>
      api.addUser({ username: username.trim(), password, role, products: [] }),
    onSuccess: () => {
      toast.success(`账号 ${username.trim()} 已创建`);
      onCreated();
      onOpenChange(false);
      setUsername("");
      setPassword("");
      setRole("owner");
      setError(null);
    },
    onError: (error) => setError(error as ApiRequestError),
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>新建账号</DialogTitle>
          <DialogDescription>
            创建后可以把具体产品授权给这个账号。管理员自动拥有全部产品。
          </DialogDescription>
        </DialogHeader>
        <form
          id="create-user"
          className="space-y-3"
          onSubmit={(event) => {
            event.preventDefault();
            setError(null);
            create.mutate();
          }}
        >
          <div className="space-y-1.5">
            <Label htmlFor="new-username">用户名</Label>
            <Input
              id="new-username"
              autoFocus
              autoComplete="off"
              value={username}
              onChange={(event) => setUsername(event.target.value)}
              aria-invalid={error?.field === "username"}
            />
            {error?.field === "username" && (
              <p className="text-destructive text-xs" role="alert">
                {error.message}
              </p>
            )}
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="new-password">初始密码</Label>
            <Input
              id="new-password"
              type="password"
              autoComplete="new-password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              aria-invalid={error?.field === "password"}
            />
            {error?.field === "password" ? (
              <p className="text-destructive text-xs" role="alert">
                {error.message}
              </p>
            ) : (
              <p className="text-muted-foreground text-xs">至少 8 位。</p>
            )}
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="new-role">角色</Label>
            <select
              id="new-role"
              className="border-input bg-background h-9 w-full rounded-md border px-2 text-sm"
              value={role}
              onChange={(event) => setRole(event.target.value)}
            >
              <option value="owner">产品负责人</option>
              <option value="admin">管理员</option>
            </select>
          </div>
          {error && !error.field && (
            <p className="text-destructive text-sm" role="alert">
              {error.message}
            </p>
          )}
        </form>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button
            type="submit"
            form="create-user"
            disabled={create.isPending || !username.trim() || !password}
          >
            {create.isPending ? "创建中…" : "创建"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function EditUserDialog({
  user,
  onOpenChange,
  onSaved,
}: {
  user: SessionUser | null;
  onOpenChange: (open: boolean) => void;
  onSaved: () => void;
}) {
  const [password, setPassword] = React.useState("");
  const [role, setRole] = React.useState("owner");
  const [products, setProducts] = React.useState<string[]>([]);
  const [error, setError] = React.useState<ApiRequestError | null>(null);

  const catalogue = useQuery({
    queryKey: ["products"],
    queryFn: ({ signal }) => api.products(signal),
    enabled: user !== null,
  });

  React.useEffect(() => {
    if (!user) return;
    setRole(user.role);
    setProducts(user.products ?? []);
    setPassword("");
    setError(null);
  }, [user]);

  const saveRole = useMutation({
    mutationFn: () => api.setRole(user!.username, role, products),
    onSuccess: () => {
      toast.success("角色与产品已更新");
      onSaved();
      onOpenChange(false);
    },
    onError: (error) => setError(error as ApiRequestError),
  });

  const savePassword = useMutation({
    mutationFn: () => api.setPassword(user!.username, password),
    onSuccess: () => {
      toast.success("密码已更新");
      setPassword("");
      onSaved();
    },
    onError: (error) => setError(error as ApiRequestError),
  });

  const available = (catalogue.data?.products ?? []).filter((row) => row.registered);

  return (
    <Dialog open={user !== null} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>编辑 {user?.username}</DialogTitle>
          <DialogDescription>
            角色决定能做什么，产品决定能管理哪些产品的发布。
          </DialogDescription>
        </DialogHeader>
        <form
          id="edit-user"
          className="space-y-4"
          onSubmit={(event) => {
            event.preventDefault();
            setError(null);
            saveRole.mutate();
          }}
        >
          <div className="space-y-1.5">
            <Label htmlFor="edit-role">角色</Label>
            <select
              id="edit-role"
              className="border-input bg-background h-9 w-full rounded-md border px-2 text-sm"
              value={role}
              onChange={(event) => setRole(event.target.value)}
            >
              <option value="owner">产品负责人</option>
              <option value="admin">管理员</option>
            </select>
          </div>

          {role !== "admin" && (
            <div className="space-y-1.5">
              <Label>可管理的产品</Label>
              {available.length === 0 ? (
                <p className="text-muted-foreground text-xs">还没有已注册的产品。</p>
              ) : (
                <div className="grid max-h-44 gap-1 overflow-y-auto rounded-md border p-2">
                  {available.map((row) => {
                    const checked = products.includes(row.id);
                    return (
                      <label
                        key={row.id}
                        className="hover:bg-accent flex cursor-pointer items-center gap-2 rounded px-1.5 py-1 text-sm"
                      >
                        <input
                          type="checkbox"
                          checked={checked}
                          onChange={(event) => {
                            setProducts((current) =>
                              event.target.checked
                                ? [...current, row.id]
                                : current.filter((id) => id !== row.id),
                            );
                          }}
                        />
                        <span className="font-mono text-xs">{row.id}</span>
                      </label>
                    );
                  })}
                </div>
              )}
            </div>
          )}

          <div className="space-y-1.5 border-t pt-3">
            <Label htmlFor="edit-password">重置密码</Label>
            <div className="flex gap-2">
              <Input
                id="edit-password"
                type="password"
                autoComplete="new-password"
                placeholder="留空则不改"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
              />
              <Button
                type="button"
                variant="outline"
                disabled={password.length < 8 || savePassword.isPending}
                onClick={() => {
                  setError(null);
                  savePassword.mutate();
                }}
              >
                重置
              </Button>
            </div>
            <p className="text-muted-foreground text-xs">至少 8 位。</p>
          </div>

          {error && (
            <p className="text-destructive text-sm" role="alert">
              {error.message}
            </p>
          )}
        </form>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button type="submit" form="edit-user" disabled={saveRole.isPending}>
            {saveRole.isPending ? "保存中…" : "保存"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
