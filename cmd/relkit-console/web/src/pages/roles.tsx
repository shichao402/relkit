import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { PageHeader } from "@/components/app-shell";
import { ErrorState } from "@/components/states";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { formatCount } from "@/lib/utils";
import { ShieldCheckIcon, UserIcon } from "lucide-react";

/** Roles are fixed by the agent (admin / owner); this page explains the two
 *  boundaries and shows how many accounts hold each. It is read-only by design:
 *  a role is not a thing you edit, it is a thing you assign on a user. */

const descriptions: Record<
  string,
  { label: string; summary: string; abilities: string[]; icon: typeof ShieldCheckIcon }
> = {
  admin: {
    label: "管理员",
    summary: "可以管理全部账户、产品与令牌，也能浏览发布树的原始文件。",
    abilities: [
      "创建、修改、删除账号并分配角色",
      "注册与下线任何产品",
      "签发、轮换、吊销任何产品的令牌",
      "浏览发布树原始键空间",
    ],
    icon: ShieldCheckIcon,
  },
  owner: {
    label: "产品负责人",
    summary: "只能管理被授权产品的事务，看不到其他产品。",
    abilities: [
      "查看被授权产品的发布状态与下载统计",
      "签发、轮换、吊销被授权产品的令牌",
      "不能创建账号、不能注册新产品",
      "不能浏览发布树原始文件",
    ],
    icon: UserIcon,
  },
};

export function RolesPage() {
  const roles = useQuery({
    queryKey: ["roles"],
    queryFn: ({ signal }) => api.roles(signal),
  });

  if (roles.isPending) {
    return (
      <>
        <PageHeader title="角色" />
        <div className="grid gap-3 sm:grid-cols-2">
          {[0, 1].map((index) => (
            <Skeleton key={index} className="h-48 w-full" />
          ))}
        </div>
      </>
    );
  }
  if (roles.error) {
    return (
      <>
        <PageHeader title="角色" />
        <ErrorState
          message={(roles.error as Error).message}
          onRetry={() => void roles.refetch()}
        />
      </>
    );
  }

  return (
    <>
      <PageHeader
        title="角色"
        description="角色由 agent 定义，决定一个账号能做什么。角色本身不可编辑，只能在用户上分配。"
      />
      <div className="grid gap-3 sm:grid-cols-2">
        {roles.data.roles.map((role) => {
          const meta = descriptions[role.name];
          const Icon = meta?.icon ?? UserIcon;
          return (
            <Card key={role.name}>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <Icon className="text-muted-foreground size-4" />
                  {meta?.label ?? role.name}
                  <Badge variant="neutral" className="num ml-auto">
                    {formatCount(role.members)} 人
                  </Badge>
                </CardTitle>
                <CardDescription>{meta?.summary}</CardDescription>
              </CardHeader>
              <CardContent>
                <ul className="text-muted-foreground space-y-1 text-sm">
                  {(meta?.abilities ?? []).map((ability) => (
                    <li key={ability} className="flex gap-2">
                      <span className="text-muted-foreground/50">·</span>
                      {ability}
                    </li>
                  ))}
                </ul>
              </CardContent>
            </Card>
          );
        })}
      </div>
    </>
  );
}
