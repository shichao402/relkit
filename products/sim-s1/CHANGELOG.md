# Changelog

## 0.1.0+6

- 第六版：e2e driver 修复（两段式 apply 按 active.json 指针断言 + 重启语义），验证 +5 → +6 真实升级路径。

## 0.1.0+5

- 第五版：+4 发布后 dev 渠道全量 verify 通过（0.1.0+3 脏节点已 unpublish），重验 from 0.1.0+4 → 0.1.0+5 真实升级路径。

## 0.1.0+4

- 第四版：启用真实 from→to 升级 e2e（baseline 从 CDN 拉取 0.1.0+3，check → download → apply → 重启断言）。

## 0.1.0+3

- 第三版：修正 e2e 基线安装的 sidecar 提取（payload zip 内 files/ 目录段），重验 from→to 升级路径。

## 0.1.0+2

- 第二版：验证 from 0.1.0+1 → 0.1.0+2 真实升级路径（dart facade check → apply → 重启断言）。

## 0.1.0+1

- 初始版本：sim-s1 独立应用自更新（dart facade，windows library + darwin in-place）。
