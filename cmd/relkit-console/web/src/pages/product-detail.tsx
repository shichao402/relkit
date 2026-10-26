import * as React from "react";
import { Link, useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, ApiRequestError } from "@/lib/api";
import type { ChannelDetail, RetireResult } from "@/lib/api-types";
import { useSession } from "@/lib/session";
import { PageHeader } from "@/components/app-shell";
import { EmptyState, ErrorState } from "@/components/states";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
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
import { formatCount, formatStamp } from "@/lib/utils";
import { ChevronRightIcon, DownloadIcon, TrashIcon } from "lucide-react";

export function ProductPage() {
  const { id = "" } = useParams();
  const { session } = useSession();
  const isAdmin = session?.user?.role === "admin";
  const client = useQueryClient();
  const [confirmOpen, setConfirmOpen] = React.useState(false);
  const [preview, setPreview] = React.useState<RetireResult | null>(null);

  const product = useQuery({
    queryKey: ["product", id],
    queryFn: ({ signal }) => api.product(id, signal),
  });

  // The preview leads: an operator sees what would go before being offered the
  // irreversible button, so the confirmation dialog is always opened with facts
  // rather than a warning about consequences in the abstract.
  const previewRemove = useMutation({
    mutationFn: () => api.deleteProduct(id, { dryRun: true }),
    onSuccess: (result) => setPreview(result),
    onError: (error) => toast.error((error as ApiRequestError).message),
  });

  const remove = useMutation({
    mutationFn: () => api.deleteProduct(id),
    onSuccess: async () => {
      toast.success(`产品 ${id} 已下线`);
      setConfirmOpen(false);
      setPreview(null);
      await client.invalidateQueries({ queryKey: ["products"] });
    },
    onError: (error) => {
      toast.error((error as ApiRequestError).message);
    },
  });

  if (product.isPending) return <ProductSkeleton />;
  if (product.error) {
    return (
      <>
        <PageHeader title={id} />
        <ErrorState
          message={(product.error as Error).message}
          onRetry={() => void product.refetch()}
        />
      </>
    );
  }

  const detail = product.data.product;

  return (
    <>
      <PageHeader
        title={detail.display || detail.id}
        description={
          <span className="flex flex-wrap items-center gap-x-3 gap-y-1">
            <span className="font-mono text-xs">{detail.id}</span>
            {detail.registered ? (
              <Badge variant="success">已注册</Badge>
            ) : (
              <Badge variant="warning">未注册的目录</Badge>
            )}
            {detail.published ? (
              <Badge variant="outline">已发布</Badge>
            ) : (
              <Badge variant="neutral">尚未发布</Badge>
            )}
            {detail.updated && <span>更新于 {formatStamp(detail.updated)}</span>}
          </span>
        }
        actions={
          <>
            <Button variant="outline" size="sm" asChild>
              <Link to={`/products/${encodeURIComponent(id)}/tokens`}>管理令牌</Link>
            </Button>
            {isAdmin && detail.registered && (
              <Button
                variant="outline"
                size="sm"
                disabled={previewRemove.isPending}
                onClick={() => previewRemove.mutate()}
              >
                <TrashIcon />
                {previewRemove.isPending ? "正在预演…" : "下线产品"}
              </Button>
            )}
          </>
        }
      />

      {detail.description && (
        <p className="text-muted-foreground mb-4 max-w-3xl text-sm">
          {detail.description}
        </p>
      )}

      {detail.root && (
        <Card className="mb-4 gap-1 py-4">
          <CardContent className="flex flex-wrap items-center gap-x-6 gap-y-1 text-sm">
            <span className="text-muted-foreground">产品根目录</span>
            <span className="font-mono text-xs break-all">{detail.root}</span>
            {detail.profile && (
              <>
                <span className="text-muted-foreground">profile</span>
                <span className="font-mono text-xs">{detail.profile}</span>
              </>
            )}
          </CardContent>
        </Card>
      )}

      {detail.detail.length === 0 ? (
        <EmptyState
          title="还没有发布记录"
          description="发布树里还没有这个产品的渠道数据。用 relkit 发布第一个版本后，这里会显示版本、产物与校验值。"
        />
      ) : (
        <div className="grid gap-3">
          {detail.detail.map((channel) => (
            <ChannelCard key={channel.name} channel={channel} />
          ))}
        </div>
      )}

      <PreviewDialog
        id={id}
        result={preview}
        onClose={() => setPreview(null)}
        onProceed={() => {
          setPreview(null);
          setConfirmOpen(true);
        }}
      />

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={`下线产品 ${id}`}
        description={
          <div className="space-y-2">
            <p>这会从 agent 中删除产品的注册信息与令牌，已发布的版本仍然留在发布树上。</p>
            <p className="text-warning">已签发的令牌立即失效，使用它的 CI 发布会失败。</p>
            <p>这个操作无法撤销，需要重新注册并重新配置 CI。</p>
          </div>
        }
        confirmLabel="下线产品"
        requirePhrase={id}
        pending={remove.isPending}
        onConfirm={() => {
          remove.mutate();
        }}
      />
      {detail.homepage && (
        <p className="text-muted-foreground mt-4 text-xs">
          主页 {detail.homepage}
        </p>
      )}
    </>
  );
}

/** Shows what a retirement would remove before the operator can commit to it.
 *  Every line comes from the agent, so the preview and the real run cannot
 *  disagree about what is on disk. */
function PreviewDialog({
  id,
  result,
  onClose,
  onProceed,
}: {
  id: string;
  result: RetireResult | null;
  onClose: () => void;
  onProceed: () => void;
}) {
  const lines = result?.log ?? [];

  return (
    <Dialog open={result !== null} onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>下线预演：{id}</DialogTitle>
          <DialogDescription>
            这一步不会改动任何东西。下面是 agent 检查发布树后，真实下线会做的事。
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-2 text-sm">
          {result?.removedTokens ? (
            <p>
              会吊销 <span className="num font-medium">{result.removedTokens}</span> 个令牌，
              并删除产品注册信息。
            </p>
          ) : (
            <p>会删除产品注册信息。该产品目前没有令牌。</p>
          )}
          {lines.length > 0 && (
            <ul className="bg-muted/40 space-y-1 rounded-md border p-3 font-mono text-xs break-all">
              {lines.map((line, index) => (
                <li key={index}>{line}</li>
              ))}
            </ul>
          )}
          {lines.some((line) => line.includes("still on disk")) && (
            <p className="text-warning">
              发布树目录不会被删除，下线后需要手工清理。
            </p>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            取消
          </Button>
          <Button variant="destructive" onClick={onProceed}>
            继续下线
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function ChannelCard({ channel }: { channel: ChannelDetail }) {
  const [showAll, setShowAll] = React.useState(false);
  const releases = showAll ? channel.releases : channel.releases.slice(0, 5);
  const hidden = channel.releases.length - releases.length;

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex flex-wrap items-center gap-2">
          {channel.name}
          {channel.latestVersion ? (
            <Badge variant="primary">{channel.latestVersion}</Badge>
          ) : (
            <Badge variant="neutral">无版本</Badge>
          )}
          {channel.latestCode ? (
            <span className="text-muted-foreground num text-xs">
              code {channel.latestCode}
            </span>
          ) : null}
        </CardTitle>
        <div className="text-muted-foreground ml-auto text-xs">
          {channel.latestReleased && <span>{formatStamp(channel.latestReleased)}</span>}
          {channel.sequence > 0 && (
            <span className="ml-2 num">序号 {channel.sequence}</span>
          )}
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        {channel.error && (
          <ErrorState message={`渠道数据读取失败：${channel.error}`} />
        )}

        {channel.artifacts.length > 0 && (
          <div>
            <h3 className="mb-2 text-sm font-medium">最新版本的产物</h3>
            <div className="grid gap-1.5">
              {channel.artifacts.map((artifact) => (
                <div
                  key={artifact.id}
                  className="bg-muted/40 flex flex-wrap items-center gap-2 rounded-md border px-3 py-2"
                >
                  <span className="min-w-0 flex-1 truncate font-mono text-xs">
                    {artifact.filename}
                  </span>
                  <Badge variant="outline">{artifact.platform}</Badge>
                  <span className="text-muted-foreground num text-xs">
                    {artifact.size}
                  </span>
                  {artifact.href && (
                    <a
                      href={artifact.href}
                      className="text-primary inline-flex items-center gap-1 text-xs hover:underline"
                    >
                      <DownloadIcon className="size-3.5" />
                      下载
                    </a>
                  )}
                </div>
              ))}
            </div>
          </div>
        )}

        {channel.releases.length > 0 && (
          <details className="group">
            <summary className="text-muted-foreground hover:text-foreground cursor-pointer text-sm select-none">
              历史版本（{formatCount(channel.releases.length)}）
              {channel.truncated ? ` 显示最近部分，已省略 ${channel.truncated}` : ""}
            </summary>
            <div className="mt-2 overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-muted-foreground border-b text-left text-xs">
                    <th className="py-1.5 pr-3 font-medium">版本</th>
                    <th className="py-1.5 pr-3 font-medium">code</th>
                    <th className="py-1.5 pr-3 font-medium">时间</th>
                    <th className="py-1.5 font-medium">状态</th>
                  </tr>
                </thead>
                <tbody>
                  {releases.map((release) => (
                    <tr key={release.version} className="border-b last:border-0">
                      <td className="py-1.5 pr-3 font-mono text-xs">{release.version}</td>
                      <td className="py-1.5 pr-3 num text-xs">{release.code}</td>
                      <td className="text-muted-foreground py-1.5 pr-3 text-xs">
                        {formatStamp(release.released)}
                      </td>
                      <td className="py-1.5">
                        {release.yanked ? (
                          <Badge variant="warning">已撤回</Badge>
                        ) : (
                          <span className="text-muted-foreground text-xs">有效</span>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            {hidden > 0 && (
              <Button
                variant="ghost"
                size="sm"
                className="mt-1"
                onClick={() => setShowAll(true)}
              >
                展开剩余 {hidden} 个
              </Button>
            )}
          </details>
        )}

        <details className="group">
          <summary className="text-muted-foreground hover:text-foreground flex cursor-pointer items-center gap-1 text-xs select-none">
            <ChevronRightIcon className="size-3.5 transition-transform group-open:rotate-90" />
            技术信息
          </summary>
          <dl className="text-muted-foreground mt-2 grid gap-1 text-xs">
            {channel.indexHref && (
              <div className="flex flex-wrap gap-2">
                <dt>索引</dt>
                <dd className="font-mono break-all">{channel.indexHref}</dd>
              </div>
            )}
            {channel.generated && (
              <div className="flex flex-wrap gap-2">
                <dt>生成于</dt>
                <dd>{formatStamp(channel.generated)}</dd>
              </div>
            )}
            {channel.latestNotes && (
              <div className="flex flex-wrap gap-2">
                <dt>发布说明</dt>
                <dd className="break-all">{channel.latestNotes}</dd>
              </div>
            )}
          </dl>
        </details>
      </CardContent>
    </Card>
  );
}

function ProductSkeleton() {
  return (
    <>
      <Skeleton className="mb-4 h-7 w-52" />
      <div className="grid gap-3">
        {[0, 1].map((index) => (
          <Card key={index}>
            <CardHeader>
              <Skeleton className="h-4 w-28" />
            </CardHeader>
            <CardContent className="space-y-2">
              <Skeleton className="h-8 w-full" />
              <Skeleton className="h-8 w-full" />
            </CardContent>
          </Card>
        ))}
      </div>
    </>
  );
}
