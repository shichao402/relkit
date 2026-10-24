import * as React from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, ApiRequestError } from "@/lib/api";
import { useSession } from "@/lib/session";
import type { IssuedToken } from "@/lib/api-types";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { OneTimeSecret } from "@/components/one-time-secret";

/** Register a product.
 *
 *  Acceptance finding #2: the old form's placeholder promised
 *  /srv/relkit/<product> on a machine whose products live under /data/relkit,
 *  and leaving the field empty made the agent try that unwritable path. The
 *  default is now the agent's own answer (requirement 4.3): this form shows the
 *  real parent directory it will use, and a failed write puts the agent's reason
 *  under the field it names. */

export function RegisterProductDialog({
  open,
  onOpenChange,
  existingIds,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  existingIds: string[];
}) {
  const { session } = useSession();
  const client = useQueryClient();
  const settings = session?.settings;
  const parent = settings?.productRootParent ?? "";

  const [id, setId] = React.useState("");
  const [root, setRoot] = React.useState("");
  const [label, setLabel] = React.useState("");
  const [fieldError, setFieldError] = React.useState<ApiRequestError | null>(null);
  const [issued, setIssued] = React.useState<IssuedToken | null>(null);

  const reset = React.useCallback(() => {
    setId("");
    setRoot("");
    setLabel("");
    setFieldError(null);
    setIssued(null);
  }, []);

  const create = useMutation({
    mutationFn: () =>
      api.addProduct({
        id: id.trim(),
        root: root.trim(),
        profile: "",
        label: label.trim(),
      }),
    onSuccess: async (payload) => {
      setIssued(payload);
      await client.invalidateQueries({ queryKey: ["products"] });
      toast.success(`产品 ${id.trim()} 已注册`);
    },
    onError: (error) => {
      setFieldError(error as ApiRequestError);
    },
  });

  // What the agent will actually use when the field stays empty. Shown live so
  // the operator sees the destination before committing, not after a failure.
  const effectiveRoot = root.trim()
    ? root.trim()
    : parent
      ? `${parent.replace(/\/+$/, "")}/${id.trim() || "<产品名>"}`
      : "";
  const rootSource =
    settings?.productRootParentSource === "config"
      ? "来自 agent 配置"
      : settings?.productRootParentSource === "inferred"
        ? "由已注册产品推断"
        : "";

  const duplicate = id.trim() !== "" && existingIds.includes(id.trim());

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) reset();
        onOpenChange(next);
      }}
    >
      <DialogContent className="sm:max-w-lg">
        {issued ? (
          <>
            <DialogHeader>
              <DialogTitle>产品已注册</DialogTitle>
              <DialogDescription>
                保存下面这个发布令牌。它只显示这一次，关闭后无法再次查看。
              </DialogDescription>
            </DialogHeader>
            <OneTimeSecret
              value={issued.plaintext ?? ""}
              label={`${issued.token.product} / ${issued.token.id}`}
            />
            <DialogFooter>
              <Button
                onClick={() => {
                  reset();
                  onOpenChange(false);
                }}
              >
                完成
              </Button>
            </DialogFooter>
          </>
        ) : (
          <>
            <DialogHeader>
              <DialogTitle>注册产品</DialogTitle>
              <DialogDescription>
                agent 会为产品生成签名密钥与发布令牌。产品名用于 URL 与发布命令，创建后不可更改。
              </DialogDescription>
            </DialogHeader>
            <form
              id="register-product"
              className="space-y-3"
              onSubmit={(event) => {
                event.preventDefault();
                setFieldError(null);
                create.mutate();
              }}
            >
              <div className="space-y-1.5">
                <Label htmlFor="product-id">
                  产品名 <span className="text-destructive">*</span>
                </Label>
                <Input
                  id="product-id"
                  autoFocus
                  autoComplete="off"
                  className="font-mono"
                  placeholder="loom"
                  value={id}
                  onChange={(event) => setId(event.target.value)}
                  aria-invalid={Boolean(fieldError?.field === "id") || duplicate}
                />
                <FieldNote error={fieldError} field="id">
                  小写字母、数字与连字符，例如 loom-launcher。
                </FieldNote>
                {duplicate && (
                  <p className="text-destructive text-xs" role="alert">
                    已经有一个同名的产品。
                  </p>
                )}
              </div>

              <div className="space-y-1.5">
                <Label htmlFor="product-root">产品根目录</Label>
                <Input
                  id="product-root"
                  autoComplete="off"
                  className="font-mono"
                  placeholder={parent ? `${parent}/<产品名>` : "留空则使用服务器默认值"}
                  value={root}
                  onChange={(event) => setRoot(event.target.value)}
                  aria-invalid={Boolean(fieldError?.field === "root")}
                />
                {effectiveRoot ? (
                  <p className="text-muted-foreground text-xs">
                    将使用 <span className="font-mono">{effectiveRoot}</span>
                    {!root.trim() && rootSource && `（${rootSource}）`}
                  </p>
                ) : (
                  <p className="text-muted-foreground text-xs">
                    agent 没有可用的默认父目录，请填写一个绝对路径。
                  </p>
                )}
                <FieldError error={fieldError} field="root" />
              </div>

              <div className="space-y-1.5">
                <Label htmlFor="product-label">令牌备注</Label>
                <Input
                  id="product-label"
                  autoComplete="off"
                  placeholder="CI 发布机"
                  value={label}
                  onChange={(event) => setLabel(event.target.value)}
                />
                <p className="text-muted-foreground text-xs">
                  写清楚这把令牌给谁用，日后轮换时便于判断。
                </p>
              </div>

              {fieldError && !fieldError.field && (
                <p className="text-destructive text-sm" role="alert">
                  {fieldError.message}
                </p>
              )}
            </form>
            <DialogFooter>
              <Button variant="outline" onClick={() => onOpenChange(false)}>
                取消
              </Button>
              <Button
                type="submit"
                form="register-product"
                disabled={create.isPending || id.trim() === "" || duplicate}
              >
                {create.isPending ? "注册中…" : "注册"}
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}

function FieldError({
  error,
  field,
}: {
  error: ApiRequestError | null;
  field: string;
}) {
  if (error?.field !== field) return null;
  return (
    <p className="text-destructive text-xs break-words" role="alert">
      {error.message}
    </p>
  );
}

function FieldNote({
  error,
  field,
  children,
}: {
  error: ApiRequestError | null;
  field: string;
  children: React.ReactNode;
}) {
  const message = error?.field === field ? error.message : null;
  return (
    <>
      {message ? (
        <p className="text-destructive text-xs break-words" role="alert">
          {message}
        </p>
      ) : (
        <p className="text-muted-foreground text-xs">{children}</p>
      )}
    </>
  );
}
