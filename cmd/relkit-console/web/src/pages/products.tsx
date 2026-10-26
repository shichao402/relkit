import * as React from "react";
import { Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import type { ProductSummary } from "@/lib/api-types";
import { PageHeader } from "@/components/app-shell";
import { EmptyState, ErrorState } from "@/components/states";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { formatCount, formatStamp } from "@/lib/utils";
import { SearchIcon, PackageOpenIcon } from "lucide-react";
import { RegisterProductDialog } from "@/components/product/register-product-dialog";

export function ProductsPage() {
  const [query, setQuery] = React.useState("");
  const [registerOpen, setRegisterOpen] = React.useState(false);
  const products = useQuery({
    queryKey: ["products"],
    queryFn: ({ signal }) => api.products(signal),
  });

  const rows = products.data?.products ?? [];
  const filtered = React.useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return rows;
    return rows.filter(
      (row) =>
        row.id.toLowerCase().includes(needle) ||
        row.display.toLowerCase().includes(needle) ||
        row.description?.toLowerCase().includes(needle),
    );
  }, [rows, query]);

  return (
    <>
      <PageHeader
        title="产品"
        description="已注册的产品及其发布状态。注册后即可用 relkit 发布第一个版本。"
        actions={
          <Button onClick={() => setRegisterOpen(true)}>注册产品</Button>
        }
      />

      <div className="mb-3 flex items-center gap-2">
        <div className="relative max-w-xs flex-1">
          <SearchIcon className="text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2" />
          <Input
            className="pl-8"
            placeholder="按名称或描述搜索"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            aria-label="搜索产品"
          />
        </div>
        <span className="text-muted-foreground ml-auto text-sm num">
          {formatCount(filtered.length)} / {formatCount(rows.length)}
        </span>
      </div>

      {products.isPending ? (
        <ProductListSkeleton />
      ) : products.error ? (
        <ErrorState
          message={(products.error as Error).message}
          onRetry={() => void products.refetch()}
        />
      ) : filtered.length === 0 ? (
        rows.length === 0 ? (
          <EmptyState
            icon={<PackageOpenIcon className="size-7" />}
            title="还没有注册任何产品"
            description="注册一个产品后，relkit 会把每个版本、渠道和平台产物记录在发布树上。"
            action={<Button onClick={() => setRegisterOpen(true)}>注册第一个产品</Button>}
          />
        ) : (
          <EmptyState
            title="没有匹配的产品"
            description={`没有产品名称包含“${query.trim()}”。`}
            action={
              <Button variant="outline" size="sm" onClick={() => setQuery("")}>
                清除搜索
              </Button>
            }
          />
        )
      ) : (
        <div className="grid gap-2.5">
          {filtered.map((row) => (
            <ProductRow key={row.id} row={row} />
          ))}
        </div>
      )}

      <RegisterProductDialog
        open={registerOpen}
        onOpenChange={setRegisterOpen}
        existingIds={rows.map((row) => row.id)}
      />
    </>
  );
}

function ProductRow({ row }: { row: ProductSummary }) {
  return (
    <Card className="gap-3 py-4 transition-colors hover:border-primary/40">
      <div className="flex flex-wrap items-start gap-3 px-5">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <Link
              to={`/products/${encodeURIComponent(row.id)}`}
              className="text-base font-medium hover:underline"
            >
              {row.display || row.id}
            </Link>
            {row.display !== row.id && (
              <span className="text-muted-foreground font-mono text-xs">{row.id}</span>
            )}
            {!row.registered && <Badge variant="warning">未注册的目录</Badge>}
            {row.registered && !row.published && <Badge variant="neutral">尚未发布</Badge>}
          </div>
          {row.description && (
            <p className="text-muted-foreground mt-1 line-clamp-1 text-sm">
              {row.description}
            </p>
          )}
          <div className="text-muted-foreground mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs">
            {row.root && <span className="font-mono break-all">{row.root}</span>}
            {row.updated && <span>更新于 {formatStamp(row.updated)}</span>}
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-1.5">
          {row.channels.length === 0 ? (
            <span className="text-muted-foreground text-xs">暂无渠道</span>
          ) : (
            row.channels.map((channel) => (
              <Badge
                key={channel.name}
                variant={channel.error ? "destructive" : channel.yanked ? "warning" : "outline"}
                title={channel.error ?? undefined}
              >
                {channel.name}
                {channel.version ? ` ${channel.version}` : ""}
              </Badge>
            ))
          )}
        </div>
      </div>
    </Card>
  );
}

function ProductListSkeleton() {
  return (
    <div className="grid gap-2.5">
      {[0, 1, 2].map((index) => (
        <Card key={index} className="gap-3 py-4">
          <div className="space-y-2 px-5">
            <Skeleton className="h-4 w-40" />
            <Skeleton className="h-3 w-64" />
          </div>
        </Card>
      ))}
    </div>
  );
}
