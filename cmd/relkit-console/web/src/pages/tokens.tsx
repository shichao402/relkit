import * as React from "react";
import { Link, useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, ApiRequestError } from "@/lib/api";
import type { IssuedToken, Token } from "@/lib/api-types";
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
import { OneTimeSecret } from "@/components/one-time-secret";
import { formatStamp } from "@/lib/utils";
import { KeyRoundIcon, PlusIcon } from "lucide-react";

/** Tokens for one product.
 *
 *  Every write here is a button, never a form submit: rotate and revoke are the
 *  two actions that made acceptance finding #3 dangerous, because a browser
 *  refresh replayed them. The plaintext of a rotate or issue lands in a dialog
 *  that shows it once and drops it; the list itself never carries it. */

export function TokenTable({ product }: { product: string }) {
  const client = useQueryClient();
  const [issueOpen, setIssueOpen] = React.useState(false);
  const [label, setLabel] = React.useState("");
  const [issued, setIssued] = React.useState<IssuedToken | null>(null);
  const [rotating, setRotating] = React.useState<Token | null>(null);
  const [revoking, setRevoking] = React.useState<Token | null>(null);

  const tokens = useQuery({
    queryKey: ["tokens", product],
    queryFn: ({ signal }) => api.tokens(product, signal),
  });

  const invalidate = () =>
    client.invalidateQueries({ queryKey: ["tokens", product] });

  const issue = useMutation({
    mutationFn: () => api.issueToken(product, label.trim()),
    onSuccess: async (payload) => {
      setIssueOpen(false);
      setLabel("");
      setIssued(payload);
      await invalidate();
    },
    onError: (error) => toast.error((error as ApiRequestError).message),
  });

  const rotate = useMutation({
    mutationFn: (token: Token) => api.rotateToken(token.id),
    onSuccess: async (payload) => {
      setRotating(null);
      setIssued(payload);
      await invalidate();
      toast.success("令牌已轮换，旧值立即失效");
    },
    onError: (error) => {
      setRotating(null);
      toast.error((error as ApiRequestError).message);
    },
  });

  const revoke = useMutation({
    mutationFn: (token: Token) => api.revokeToken(token.id),
    onSuccess: async () => {
      setRevoking(null);
      await invalidate();
      toast.success("令牌已吊销");
    },
    onError: (error) => {
      setRevoking(null);
      toast.error((error as ApiRequestError).message);
    },
  });

  if (tokens.isPending) {
    return (
      <div className="space-y-2">
        {[0, 1].map((index) => (
          <Skeleton key={index} className="h-14 w-full" />
        ))}
      </div>
    );
  }
  if (tokens.error) {
    return (
      <ErrorState
        message={(tokens.error as Error).message}
        onRetry={() => void tokens.refetch()}
      />
    );
  }

  const rows = tokens.data.tokens;

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-2">
        <p className="text-muted-foreground text-sm">
          令牌用于 CI 发布。明文只在签发或轮换时显示一次。
        </p>
        <Button size="sm" onClick={() => setIssueOpen(true)}>
          <PlusIcon />
          签发令牌
        </Button>
      </div>

      {rows.length === 0 ? (
        <EmptyState
          icon={<KeyRoundIcon className="size-7" />}
          title="还没有令牌"
          description="签发一个令牌放进 CI 的密钥里，relkit 才能代表这个产品发布。"
          action={
            <Button size="sm" onClick={() => setIssueOpen(true)}>
              签发第一个令牌
            </Button>
          }
        />
      ) : (
        <div className="grid gap-2">
          {rows.map((token) => (
            <Card key={token.id} className="gap-2 py-3">
              <div className="flex flex-wrap items-center gap-2 px-5">
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-mono text-xs">{token.id}</span>
                    {token.revokedAt ? (
                      <Badge variant="destructive">已吊销</Badge>
                    ) : (
                      <Badge variant="success">有效</Badge>
                    )}
                    {token.label && (
                      <span className="text-muted-foreground text-xs">{token.label}</span>
                    )}
                  </div>
                  <div className="text-muted-foreground mt-1 flex flex-wrap gap-x-4 text-xs">
                    <span>创建 {formatStamp(token.createdAt)}</span>
                    {token.rotatedAt && <span>轮换 {formatStamp(token.rotatedAt)}</span>}
                    {token.revokedAt && <span>吊销 {formatStamp(token.revokedAt)}</span>}
                  </div>
                </div>
                {!token.revokedAt && (
                  <div className="flex gap-1.5">
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => setRotating(token)}
                    >
                      轮换
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="text-destructive hover:bg-destructive/10"
                      onClick={() => setRevoking(token)}
                    >
                      吊销
                    </Button>
                  </div>
                )}
              </div>
            </Card>
          ))}
        </div>
      )}

      <Dialog open={issueOpen} onOpenChange={setIssueOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>签发令牌</DialogTitle>
            <DialogDescription>
              为 {product} 签发一把新令牌。明文只会显示一次。
            </DialogDescription>
          </DialogHeader>
          <form
            id="issue-token"
            className="space-y-3"
            onSubmit={(event) => {
              event.preventDefault();
              issue.mutate();
            }}
          >
            <div className="space-y-1.5">
              <Label htmlFor="token-label">备注</Label>
              <Input
                id="token-label"
                autoFocus
                autoComplete="off"
                placeholder="内网蓝盾发布"
                value={label}
                onChange={(event) => setLabel(event.target.value)}
              />
              <p className="text-muted-foreground text-xs">
                写明这把令牌给哪个环境用，日后轮换时能判断影响范围。
              </p>
            </div>
          </form>
          <DialogFooter>
            <Button variant="outline" onClick={() => setIssueOpen(false)}>
              取消
            </Button>
            <Button
              type="submit"
              form="issue-token"
              disabled={issue.isPending}
            >
              {issue.isPending ? "签发中…" : "签发"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={issued !== null} onOpenChange={(open) => !open && setIssued(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>令牌明文</DialogTitle>
            <DialogDescription>
              复制并保存到安全的地方。关闭这个对话框后无法再次查看。
            </DialogDescription>
          </DialogHeader>
          {issued && (
            <OneTimeSecret
              value={issued.plaintext ?? ""}
              label={`${issued.token.product} / ${issued.token.id}`}
            />
          )}
          <DialogFooter>
            <Button onClick={() => setIssued(null)}>我已保存</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={rotating !== null}
        onOpenChange={(open) => !open && setRotating(null)}
        title="轮换令牌"
        description={
          <div className="space-y-1.5">
            <p>令牌 {rotating?.id} 的旧值立即失效，正在使用它的发布任务会失败。</p>
            <p>新明文只显示一次，记得同步更新 CI 里的密钥。</p>
          </div>
        }
        confirmLabel="轮换"
        pending={rotate.isPending}
        onConfirm={() => rotating && rotate.mutate(rotating)}
      />

      <ConfirmDialog
        open={revoking !== null}
        onOpenChange={(open) => !open && setRevoking(null)}
        title="吊销令牌"
        description={
          <div className="space-y-1.5">
            <p>吊销后令牌 {revoking?.id} 立刻无法使用，且不能再恢复。</p>
            <p>已吊销的令牌会保留在列表里，便于追溯是谁在什么时候撤掉了它。</p>
          </div>
        }
        confirmLabel="吊销"
        pending={revoke.isPending}
        onConfirm={() => revoking && revoke.mutate(revoking)}
      />
    </div>
  );
}

export function ProductTokensPage() {
  const { id = "" } = useParams();
  return (
    <>
      <PageHeader
        title={`${id} 的令牌`}
        description={
          <span>
            管理 {id} 的发布令牌。返回{" "}
            <Link
              to={`/products/${encodeURIComponent(id)}`}
              className="text-primary hover:underline"
            >
              产品详情
            </Link>
            。
          </span>
        }
      />
      <TokenTable product={id} />
    </>
  );
}

/** The cross-product view: every product's tokens in one list, for the operator
 *  who needs to find a stray credential rather than manage one product. */
export function TokensPage() {
  const products = useQuery({
    queryKey: ["products"],
    queryFn: ({ signal }) => api.products(signal),
  });

  const registered = (products.data?.products ?? []).filter((row) => row.registered);

  if (products.isPending) {
    return (
      <>
        <PageHeader title="令牌" />
        <Skeleton className="h-32 w-full" />
      </>
    );
  }
  if (products.error) {
    return (
      <>
        <PageHeader title="令牌" />
        <ErrorState
          message={(products.error as Error).message}
          onRetry={() => void products.refetch()}
        />
      </>
    );
  }

  return (
    <>
      <PageHeader
        title="令牌"
        description="按产品管理发布令牌。选择一个产品查看、签发、轮换或吊销。"
      />
      {registered.length === 0 ? (
        <EmptyState
          icon={<KeyRoundIcon className="size-7" />}
          title="还没有已注册的产品"
          description="令牌属于产品，先注册一个产品再来签发令牌。"
          action={
            <Button asChild size="sm">
              <Link to="/products">去注册产品</Link>
            </Button>
          }
        />
      ) : (
        <div className="grid gap-3">
          {registered.map((row) => (
            <Card key={row.id} className="gap-3 py-4">
              <div className="flex flex-wrap items-center gap-2 px-5">
                <span className="flex-1 text-sm font-medium">{row.display || row.id}</span>
                <Button variant="outline" size="sm" asChild>
                  <Link to={`/products/${encodeURIComponent(row.id)}/tokens`}>
                    管理令牌
                  </Link>
                </Button>
              </div>
            </Card>
          ))}
        </div>
      )}
    </>
  );
}
