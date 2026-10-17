# Changelog

## 0.1.0+8

- 第八版：修复后真实升级验证——e2e from 0.1.0+7（新 codeOf，code=7）→ to 0.1.0+8，验证 build 号 code 空间下的完整 check→download→apply 升级链。

## 0.1.0+7

- 第七版（修复轮）：客户端 codeOf 语义修复——当前 code 改取版本 build 号 `+N`（与 s1/manager 同语义），此前误用 `sdk.SemverCode`（剥掉 `+N` 导致 CurrentCode=1000 恒大于索引节点，永远 up-to-date）。

## 0.1.0+6

- 第六版：from 0.1.0+5 → to 0.1.0+6 真实升级 e2e（baseline +5 制品已确认在 CDN 服务器；本轮验证组件套件 payload 升级链 +6）。

## 0.1.0+5

- 第五版：control-plane 换根（relkit-distribution-2027）+ relkit v0.6.17 控制面后重跑 dev 渠道发布与 e2e（from 0.1.0+4 → 0.1.0+5，组件套件 payload 链）。

## 0.1.0+4

- 第四版：from→to 升级 e2e（baseline 0.1.0+3，组件套件 payload 升级链 +4）。

## 0.1.0+3

- 第三版：from→to 升级 e2e（baseline 0.1.0+2，组件套件 payload 平铺安装 + check → apply → 断言）。

## 0.1.0+2

- e2e 修复轮：baseline 安装平铺解压 payload（修复 files/ 目录前缀导致组件落位错误）。

## 0.1.0+1

- 初始版本：sim-s3 可选产品套件（go facade，sim-server/sim-exec/sim-host-setup 三组件，component × audience 选择器）。

