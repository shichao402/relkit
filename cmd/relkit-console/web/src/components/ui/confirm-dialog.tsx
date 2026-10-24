import * as React from "react";
import * as AlertDialogPrimitive from "@radix-ui/react-alert-dialog";
import { cn } from "@/lib/utils";
import { buttonVariants } from "@/components/ui/button";
import { useReturnFocus } from "@/components/ui/focus-return";

/** Confirmation for a destructive action.
 *
 *  Two strengths, both here so nobody invents a third: a plain confirm that
 *  states the consequence, and `requirePhrase` for irreversible actions where
 *  typing the name is the only way to be sure the operator meant this row. */

const AlertDialog = AlertDialogPrimitive.Root;
const AlertDialogTrigger = AlertDialogPrimitive.Trigger;

function AlertDialogContent({
  className,
  onCloseAutoFocus,
  ...props
}: React.ComponentProps<typeof AlertDialogPrimitive.Content>) {
  // Same reason as DialogContent: without a trigger Radix has nowhere to send
  // focus back to. See useReturnFocus.
  const returnFocus = useReturnFocus();

  return (
    <AlertDialogPrimitive.Portal>
      <AlertDialogPrimitive.Overlay className="fixed inset-0 z-50 bg-black/55" />
      <AlertDialogPrimitive.Content
        className={cn(
          "bg-card fixed top-1/2 left-1/2 z-50 grid w-full max-w-[calc(100%-2rem)] -translate-x-1/2 -translate-y-1/2 gap-4 rounded-lg border p-6 shadow-lg sm:max-w-md",
          className,
        )}
        onCloseAutoFocus={(event) => {
          onCloseAutoFocus?.(event);
          if (event.defaultPrevented) return;
          returnFocus(event);
        }}
        {...props}
      />
    </AlertDialogPrimitive.Portal>
  );
}

function AlertDialogTitle(
  props: React.ComponentProps<typeof AlertDialogPrimitive.Title>,
) {
  return (
    <AlertDialogPrimitive.Title
      className="text-base leading-none font-semibold"
      {...props}
    />
  );
}

function AlertDialogDescription(
  props: React.ComponentProps<typeof AlertDialogPrimitive.Description>,
) {
  return (
    <AlertDialogPrimitive.Description
      className="text-muted-foreground text-sm"
      {...props}
    />
  );
}

function AlertDialogAction({
  className,
  variant = "destructive",
  ...props
}: React.ComponentProps<typeof AlertDialogPrimitive.Action> & {
  variant?: "default" | "destructive";
}) {
  return (
    <AlertDialogPrimitive.Action
      className={cn(buttonVariants({ variant }), className)}
      {...props}
    />
  );
}

function AlertDialogCancel({
  className,
  ...props
}: React.ComponentProps<typeof AlertDialogPrimitive.Cancel>) {
  return (
    <AlertDialogPrimitive.Cancel
      className={cn(buttonVariants({ variant: "outline" }), className)}
      {...props}
    />
  );
}

interface ConfirmDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: React.ReactNode;
  confirmLabel?: string;
  cancelLabel?: string;
  variant?: "default" | "destructive";
  /** When set, the operator must type this string to enable the confirm button. */
  requirePhrase?: string;
  onConfirm: () => void;
  pending?: boolean;
}

export function ConfirmDialog({
  open,
  onOpenChange,
  title,
  description,
  confirmLabel = "确认",
  cancelLabel = "取消",
  variant = "destructive",
  requirePhrase,
  onConfirm,
  pending = false,
}: ConfirmDialogProps) {
  const [typed, setTyped] = React.useState("");

  React.useEffect(() => {
    if (!open) setTyped("");
  }, [open]);

  const blocked = requirePhrase !== undefined && typed !== requirePhrase;

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogTitle>{title}</AlertDialogTitle>
        <AlertDialogDescription asChild>
          <div className="space-y-3">
            <div>{description}</div>
            {requirePhrase !== undefined && (
              <div className="space-y-1.5">
                <label
                  className="text-foreground text-xs"
                  htmlFor="confirm-phrase"
                >
                  输入 <span className="font-mono font-semibold">{requirePhrase}</span> 以确认
                </label>
                <input
                  id="confirm-phrase"
                  autoComplete="off"
                  className="border-input bg-background h-8 w-full rounded-md border px-2 font-mono text-sm"
                  value={typed}
                  onChange={(event) => setTyped(event.target.value)}
                />
              </div>
            )}
          </div>
        </AlertDialogDescription>
        <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
          <AlertDialogCancel disabled={pending}>{cancelLabel}</AlertDialogCancel>
          <AlertDialogAction
            variant={variant}
            disabled={blocked || pending}
            onClick={(event) => {
              // Radix closes on click; keep it open while the request runs so
              // the pending state is visible and a double click cannot re-fire.
              event.preventDefault();
              onConfirm();
            }}
          >
            {pending ? "处理中…" : confirmLabel}
          </AlertDialogAction>
        </div>
      </AlertDialogContent>
    </AlertDialog>
  );
}

export { AlertDialog, AlertDialogTrigger };
