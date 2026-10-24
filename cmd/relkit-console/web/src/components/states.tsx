import * as React from "react";
import { Button } from "@/components/ui/button";
import { AlertTriangleIcon, InboxIcon, RotateCwIcon } from "lucide-react";
import { cn } from "@/lib/utils";

/** The three states every data view needs, in one place so they look the same
 *  everywhere: nothing yet, nothing matched, and something broke. */

export function EmptyState({
  title,
  description,
  action,
  icon,
  className,
}: {
  title: string;
  description?: React.ReactNode;
  action?: React.ReactNode;
  icon?: React.ReactNode;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "flex flex-col items-center justify-center gap-3 rounded-lg border border-dashed px-6 py-12 text-center",
        className,
      )}
    >
      <div className="text-muted-foreground/70">
        {icon ?? <InboxIcon className="size-7" />}
      </div>
      <div className="space-y-1">
        <p className="text-sm font-medium">{title}</p>
        {description && (
          <p className="text-muted-foreground mx-auto max-w-sm text-sm">
            {description}
          </p>
        )}
      </div>
      {action}
    </div>
  );
}

/** An error keeps the reason verbatim and offers a retry: a panel that hides
 *  why a call failed cannot be used to fix anything. */
export function ErrorState({
  message,
  onRetry,
  className,
}: {
  message: string;
  onRetry?: () => void;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "border-destructive/30 bg-destructive/5 flex flex-col gap-3 rounded-lg border px-4 py-4",
        className,
      )}
    >
      <div className="flex items-start gap-2">
        <AlertTriangleIcon className="text-destructive mt-0.5 size-4 shrink-0" />
        <p className="text-sm break-words">{message}</p>
      </div>
      {onRetry && (
        <div>
          <Button variant="outline" size="sm" onClick={onRetry}>
            <RotateCwIcon />
            重试
          </Button>
        </div>
      )}
    </div>
  );
}
