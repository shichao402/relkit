# Changelog

## 0.1.0+6

- 第六版：control-plane 换根（relkit-distribution-2027）+ relkit v0.6.17 updater（in-place 跨卷 backup 修复）后重跑 from 0.1.0+5 → 0.1.0+6 真实升级 e2e（双订阅 apply + managed library 断言）。

## 0.1.0+5

- 第五版：+4 发布验证通过（in-place apply 语义已入 e2e driver），from→to 升级 e2e（baseline 0.1.0+4）。

## 0.1.0+4

- 第四版：+3 发布成功（污染节点已 unpublish），from→to 升级 e2e（baseline 0.1.0+3，双订阅 apply + managed library 断言）。

## 0.1.0+3

- 第三版：from→to 升级 e2e（baseline 0.1.0+2，双订阅 apply + 代管 managed library 断言按 active 指针）。

## 0.1.0+2

- 第二版：from→to 升级 e2e（baseline 0.1.0+1，双订阅 apply：自更新 in-place + 代管 managed library 安装）。

## 0.1.0+1

- 初始版本：sim-s2-manager 管理器（rust facade，双订阅自更新 + 代管 sim-s2-managed）。
