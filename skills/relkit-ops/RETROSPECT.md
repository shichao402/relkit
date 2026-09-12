# 完成后自省

宣称完成前执行。本文件不是手册，不要复述 SKILL.md。

1. **脚本 ↔ Skill**：Skill 声明的入口是否仍是 `relkit_host.py`（产品）和 `deploy/relkit.py` 的 `build|install|upgrade`（装机）。仍写已删入口则当场改。
2. **去冗余**：能指向 `--help` / `explain` / `.relkit/onboarding.json` 的不要抄。产品状态禁止进 skill cache。
3. **config-backed**：`ssh.host` 是否要求读 `~/.ssh/config` + Include + 通配展开，且与 `ssh_host_allowed` / `ssh_host_recommend` 一致。只列精确 Host、忽略 Include/通配则当场改 Skill。
4. **本文件**：只留检查项。不写流水账。

改动跟同一提交走。
