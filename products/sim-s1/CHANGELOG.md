# Changelog

## 0.1.0+3

- 第三版：修正 e2e 基线安装的 sidecar 提取（payload zip 内 files/ 目录段），重验 from→to 升级路径。

## 0.1.0+2

- 第二版：验证 from 0.1.0+1 → 0.1.0+2 真实升级路径（dart facade check → apply → 重启断言）。

## 0.1.0+1

- 初始版本：sim-s1 独立应用自更新（dart facade，windows library + darwin in-place）。
