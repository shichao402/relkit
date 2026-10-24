import * as React from "react";
import { CheckIcon, CopyIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

/** A credential shown exactly once.
 *
 *  Acceptance finding #3: the old panel rendered the plaintext into the POST
 *  response, so a refresh re-ran the rotation and invalidated what was on
 *  screen. Here the plaintext arrives with the call that issued it, lives only
 *  in component state, and is gone when the dialog closes. It is never put in
 *  the URL, in localStorage, or in a query cache entry, so it cannot come back
 *  from history (requirement 5.2). */

export function OneTimeSecret({
  value,
  label,
}: {
  value: string;
  label?: string;
}) {
  const [copied, setCopied] = React.useState(false);
  const [copyFailed, setCopyFailed] = React.useState(false);

  React.useEffect(() => {
    if (!copied) return;
    const timer = setTimeout(() => setCopied(false), 2000);
    return () => clearTimeout(timer);
  }, [copied]);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      setCopyFailed(false);
    } catch {
      // A clipboard permission can be refused; the value is still selectable,
      // so say so instead of failing silently.
      setCopyFailed(true);
    }
  };

  return (
    <div className="space-y-2">
      {label && (
        <p className="text-muted-foreground font-mono text-xs break-all">{label}</p>
      )}
      <div className="bg-muted/60 flex items-start gap-2 rounded-md border p-2.5">
        <code className="min-w-0 flex-1 font-mono text-xs break-all select-all">
          {value}
        </code>
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => void copy()}
          className="shrink-0"
        >
          {copied ? <CheckIcon /> : <CopyIcon />}
          {copied ? "已复制" : "复制"}
        </Button>
      </div>
      <p className={cn("text-xs", copyFailed ? "text-destructive" : "text-warning")}>
        {copyFailed
          ? "浏览器拒绝了剪贴板访问，请手动选中上面的内容复制。"
          : "只显示这一次。关闭后无法再次查看，只能轮换出新令牌。"}
      </p>
    </div>
  );
}
