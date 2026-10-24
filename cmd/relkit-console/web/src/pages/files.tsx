import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { PageHeader } from "@/components/app-shell";
import { EmptyState, ErrorState } from "@/components/states";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { formatBytes, formatStamp } from "@/lib/utils";
import { ArrowUpIcon, FileIcon, FolderIcon, FolderTreeIcon } from "lucide-react";

/** Raw key space, read-only.
 *
 *  This is the escape hatch an admin needs when the rendered views disagree with
 *  the tree: manifests, indexes and signatures are keys, not products. Owner
 *  accounts are refused by the API, and the sidebar hides the entry. */

export function FilesPage() {
  const [dir, setDir] = React.useState(".");

  const listing = useQuery({
    queryKey: ["files", dir],
    queryFn: ({ signal }) => api.files(dir, signal),
  });

  const segments = dir === "." ? [] : dir.split("/");
  const entries = listing.data?.entries ?? [];

  return (
    <>
      <PageHeader
        title="文件"
        description="发布树的原始键空间，只读。用于核对清单、索引与签名，不能在这里修改任何东西。"
      />

      <div className="mb-3 flex flex-wrap items-center gap-1 text-sm">
        <Button
          variant="ghost"
          size="sm"
          disabled={dir === "."}
          onClick={() => setDir(".")}
        >
          <FolderTreeIcon />
          根目录
        </Button>
        {segments.map((segment, index) => {
          const target = segments.slice(0, index + 1).join("/");
          return (
            <React.Fragment key={target}>
              <span className="text-muted-foreground/50">/</span>
              <Button
                variant="ghost"
                size="sm"
                className="font-mono text-xs"
                onClick={() => setDir(target)}
              >
                {segment}
              </Button>
            </React.Fragment>
          );
        })}
        {dir !== "." && (
          <Button
            variant="outline"
            size="sm"
            className="ml-auto"
            onClick={() => setDir(segments.slice(0, -1).join("/") || ".")}
          >
            <ArrowUpIcon />
            上一级
          </Button>
        )}
      </div>

      {listing.isPending ? (
        <div className="space-y-2">
          {[0, 1, 2, 3].map((index) => (
            <Skeleton key={index} className="h-9 w-full" />
          ))}
        </div>
      ) : listing.error ? (
        <ErrorState
          message={(listing.error as Error).message}
          onRetry={() => void listing.refetch()}
        />
      ) : entries.length === 0 ? (
        <EmptyState
          icon={<FolderTreeIcon className="size-7" />}
          title="这个目录是空的"
          description="没有键落在这个前缀下。发布第一个版本后，这里会出现 manifest、index 与签名文件。"
        />
      ) : (
        <Card className="gap-0 overflow-hidden py-0">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-muted-foreground bg-muted/40 border-b text-left text-xs">
                <th className="px-4 py-2 font-medium">名称</th>
                <th className="px-4 py-2 font-medium">大小</th>
                <th className="px-4 py-2 font-medium">修改时间</th>
              </tr>
            </thead>
            <tbody>
              {entries.map((entry) => (
                <tr
                  key={entry.name}
                  className="hover:bg-accent/60 border-b last:border-0"
                >
                  <td className="px-4 py-1.5">
                    {entry.dir ? (
                      <button
                        className="text-primary inline-flex items-center gap-1.5 font-mono text-xs hover:underline"
                        onClick={() =>
                          setDir(dir === "." ? entry.name : `${dir}/${entry.name}`)
                        }
                      >
                        <FolderIcon className="size-3.5" />
                        {entry.name}/
                      </button>
                    ) : (
                      <span className="inline-flex items-center gap-1.5 font-mono text-xs">
                        <FileIcon className="text-muted-foreground size-3.5" />
                        {entry.name}
                      </span>
                    )}
                  </td>
                  <td className="text-muted-foreground num px-4 py-1.5 text-xs">
                    {entry.dir ? <Badge variant="neutral">目录</Badge> : formatBytes(entry.size)}
                  </td>
                  <td className="text-muted-foreground px-4 py-1.5 text-xs">
                    {formatStamp(entry.mtime)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </Card>
      )}
    </>
  );
}
