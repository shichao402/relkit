# Production escape registry

本目录只登记“现有自动化未在生产路径前拦住”的 Relkit 通用缺陷。产品业务问题留在
宿主仓；普通开发 bug 不进入这里。

处理顺序：

1. 先写会失败的真实回归测试；跨组件缺陷必须有 scenario。
2. 立即增加一个 JSON 记录并关联 issue 或修复 PR，不等接入任务结束。
3. PR 运行 `relkit-ops escape audit --baseline-ref origin/<base> --execute`。
4. merge 后补完整 fix commit 与首次正式 release；stable tag 前不得有 open 记录。

`platforms` 只在测试确实依赖操作系统时填写，值限 `darwin`、`linux`、`windows`。
未填写表示三个平台都应执行；zero-match 和 skip 都不算通过。

字段、状态、源码摘要、Git 历史与执行判定均以 `relkit-ops escape audit` 为准。参考同目录
现有记录，不在文档或 Skill 中复制另一套 schema。
