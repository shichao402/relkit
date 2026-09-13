# 完成后自省

宣称完成前执行。本文件不是手册，不要复述 SKILL.md。

1. **脚本 ↔ Skill**：入口仍是产品仓 `relkit_host.py`、装机 `deploy/relkit.py build|install|upgrade`。仍写已删入口则当场改。
2. **去冗余**：能指向 `--help` / `explain` / `.relkit/onboarding.json` 的不要抄。产品状态禁止进 skill cache。
3. **config-backed**：`ssh.host` 是否要求读 `~/.ssh/config` + Include + 通配展开，且与 `ssh_host_allowed` / `ssh_host_recommend` 一致。
4. **开箱前**：是否先环境检查再问 `product.id`。把已有仓当绿地、漏掉 `http-put` 残留则当场改 Skill。
5. **updater JSON**：`onboard explain updater.process` 是否还写「开工窄桥」；封闭词是否仍是 `rust`（不是 `rust-shell`）；是否把缺 `releaseNotesMarkdown` 当成向后兼容。仍写窄桥或教人 `serde(default)` 则当场改 explain / Skill。
6. **缓存与 sidecar**：Skill 是否仍指向提交 `onboarding.json`、瞬时 `.relkit/cache/`、`ensure_gitignore`、dummy staged 在真发前清理、`tools/bin` + 可选 `packScript`（无 `.mjs` 硬编码）。缺则补短指针。
7. **发布与 token**：Skill 是否仍指向 `RELKIT_RELEASE_VIA_CI`、`share-with` 用 owner 的 `{owner}.token` 且不打印新 token。缺则补短指针。
8. **本文件**：只留检查项。不写流水账。

改动跟同一提交走。
