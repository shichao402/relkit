<!-- 本文件由 `dec pull` 从 .dec/cache/agents-board/ 渲染生成，请勿直接编辑。
     修改流程：编辑 .dec/cache/agents-board/... → dec push → dec pull 验证 -->

---
name: agents-board
description: >
  跨设备共享白板（tldraw 私有部署，http://board.firoyang.com）的 Agent MCP。
  用 board_* 工具创建/读写房间与图形；平板与 Cursor 侧栏实时同步。
  读板优先 board_describe_room；手写/视觉标注用 board_screenshot + 多模态；反馈标注用 board_annotate。
  当用户说画一下 / 打开白板 / 放到 board 上 / 画个思维导图 / 画流程图 / 一起看图标注时使用。
  密码已写死在 MCP 代码中，无需 mise / MCP JSON 注入。
---

# Agents Board（共享白板）

Agent 与真人共用同一块 tldraw 白板（私有部署）。Web：<http://board.firoyang.com/>

## 何时使用

1. 需要和用户（平板 / 手机 / 浏览器）一起看图、改草图、标注 UI
2. 用户说「打开白板」「画一下」「放到 board 上」「画个思维导图」
3. 跨设备联调布局、流程图、思维导图、登录页草图
4. 讨论产物需要**长期沉淀且反复深化**——白板比 Markdown 更适合当讨论主干
5. 需要读用户手写、红框、圈选时（结构化 JSON 读不懂 → 截图后用视觉）

## 鉴权

- Web 登录密码 = API Bearer token，已写死在 MCP `server.ts` 的 `TOKEN` 常量（与 `common/config.env` 一致）
- **不要**把密码写进 MCP JSON / mise env / 本文档
- Agent 修改者显示为 `<当前项目目录名> / Cursor Agent`；网页编辑显示为 `真人`

## 推荐工作流

1. `board_list_rooms` — 看现有房间
2. 没有合适房间 → `board_create_room`
3. **`board_describe_room`** — 默认读板（纯文本标签 + 空间顺序 + tips）
4. 若含 `draw`/`highlight`、或描述仍看不清 → **`board_screenshot`**，用返回的 PNG 做视觉阅读
5. 反馈/圈选 → **`board_annotate`**（`meta.role=agent-marker`，不改用户图形）
6. 真正画内容 → `board_upsert_shapes` / `board_patch_shapes`
7. 清理自己的圈选 → `board_clear_markers`
8. 把永久链接发给用户：`http://board.firoyang.com/r/<roomId>`
   - 别名：`/n/<显示名>`（改名后别名跟随变化，roomId 永远不变）

## Tools

| Tool | 作用 |
|------|------|
| `board_list_rooms` | 列出房间（名称、时间、修改者、链接） |
| `board_create_room` | 创建房间，返回永久 `roomId` 与别名 `slug` |
| `board_rename_room` | 重命名（`roomId` 不变，`slug` 跟随） |
| `board_describe_room` | **主读取**：结构化摘要 + markdown |
| `board_get_room` | 同 describe；`format=raw` 才给原始 props（调试） |
| `board_screenshot` | **内容贴合 PNG**（可 bbox / shapeIds）；手写靠多模态看图 |
| `board_annotate` | Agent 标记层：highlight / callout / arrow-point |
| `board_clear_markers` | 删除全部 agent-marker |
| `board_upsert_shapes` | 创建 / 更新 `geo` `text` `note` `arrow` |
| `board_patch_shapes` | 改位置 / 文本 / 属性 |
| `board_delete_shapes` | 删除图形（慎用用户内容） |

---

## 读写约定（重要）

- **不要**把 `board_get_room` 的 raw props / `richText` AST 当主要理解来源
- 手写笔迹（`type=draw`）**无法 OCR**；描述里会标 `readable: false`，必须 `board_screenshot`
- `board_screenshot` 默认隐藏 agent-marker；需要时再 `includeMarkers=true`
- 截图是 **content-fit**：只包住目标形状/区域 + padding，默认最长边约 1280px
- 临时反馈用 `board_annotate`，不要直接改用户 shape
- 小改优先 `patch`；大块重画再用 `upsert`

---

## 硬性约束（都是实测踩出来的，违反会直接 500 或炸上下文）

### 1. shape id 必须以 `shape:` 开头

```text
"id": "my-box"        → 500  shape ID must start with "shape:"
"id": "shape:my-box"  → OK
```

### 2. `arrow` 建不出来，用细长矩形代替

后端 tldraw 要求 `arrow.props.kind` 为 `arc` 或 `elbow`，但 `board_upsert_shapes`
的 schema 只暴露 `x/y/w/h/color/fill/size/geo/text/rotation/meta`，传不进 `props`：

```text
"type": "arrow"  → 500  At shape(type = arrow).props.kind: Expected "arc" or "elbow", got undefined
```

替代方案：用高 4~6 px 的 `geo rectangle` 当连线，`color: "grey"`、`fill: "solid"`。

**代价**：这些不是真箭头，**不会跟随节点移动**。用户在平板上拖动节点后，连线要由
Agent 重算坐标 `board_patch_shapes` 修正。画之前先跟用户说明这一点。

`board_annotate` 的 `arrow-point` 若遇同类错误，改用 `highlight`/`callout`。

### 3. 写操作会回吐房间摘要，大批量仍建议走 HTTP API

`board_upsert_shapes` / `board_patch_shapes` / `board_delete_shapes` 现在默认回吐
**describe 摘要**（比旧版 raw props 轻），但房间很大时仍会占上下文。

- **少量图形（≤ 20 个）**：正常用 MCP 工具
- **大批量 / 反复重画**：绕过 MCP，直接打后端 HTTP API，自己控制输出

```powershell
# 1) 把 {"shapes":[...]} 写进临时文件，避免 PowerShell 引号地狱
$json  = [System.IO.File]::ReadAllText("$env:TEMP\board-payload.json")
$bytes = [System.Text.Encoding]::UTF8.GetBytes($json)   # 必须转 UTF-8 字节，否则中文乱码
$r = Invoke-RestMethod -Uri "http://board.firoyang.com/api/rooms/<roomId>/shapes" `
  -Method Post `
  -Headers @{ Authorization = "Bearer <见 server.ts TOKEN>"; "X-Board-Actor" = "<项目名> / Cursor Agent" } `
  -ContentType "application/json; charset=utf-8" -Body $bytes
"提交 $($r.ids.Count) 个，房间共 $($r.shapeCount) 个"
```

对应端点（MCP 只是它的薄代理）：

| 方法 | 路径 |
|------|------|
| GET | `/api/rooms` |
| POST | `/api/rooms`（body `{name}`） |
| GET / PATCH | `/api/rooms/{roomId}` |
| GET | `/api/rooms/{roomId}/describe` |
| GET/POST | `/api/rooms/{roomId}/screenshot` |
| POST | `/api/rooms/{roomId}/annotate` |
| DELETE | `/api/rooms/{roomId}/markers` |
| POST / PATCH / DELETE | `/api/rooms/{roomId}/shapes` |

---

## 图形语义（实测）

- `x` / `y` = **左上角**坐标；画布无限，可用负数
- `geo`：`w` / `h` 生效，文本自动居中并在框内换行 → **需要可控宽度时一律用 `geo`**
- `text`：`autoSize: true`，`w` 不生效，长文本会拉成极宽的一行，排版时慎用
- `note`：固定尺寸便签，`w` / `h` 被忽略
- `fill`：`solid`（淡色填充 + 同色描边，衬黑字可读）/ `none`（透明 + 描边）/ `semi` / `pattern`
- `size`：`s` `m` `l` `xl`
- `geo` 形状：`rectangle` `ellipse` `triangle` `diamond` `pentagon` `hexagon` `cloud` `star` `check-box` `x-box`
- 颜色（tldraw 调色板）：`black` `grey` `blue` `light-blue` `violet` `light-violet`
  `green` `light-green` `yellow` `orange` `red` `light-red`

---

## 配方：画一棵思维导图 / 逻辑树

右向树最好读，也最好算。一套参数：

| 层 | x | 宽 | 高 | 样式 |
|----|----|----|----|------|
| 根 | 0 | 340 | 120 | `fill: solid`，`size: l` |
| 一级 | 520 | 340 | 76 | `fill: solid`，`size: m`，每枝一个颜色 |
| 二级 | 980 | 560 | 72 | `fill: none`，`size: s`，颜色随父枝 |

纵向：二级节点 **pitch = 88**（高 72 + 间隙 16），相邻主枝之间再留 **70** 空隙。

```text
每枝子树高 = 子节点数 × 88
该枝一级节点的 y = 子树纵向中点 − 38
根节点 y = 全图纵向中点 − 60
```

连线（细矩形）：

- 主脊：`x=430, w=6`，从第一枝一级节点中心纵坐标延伸到最后一枝
- 根 → 主脊：`x=340, w=96, h=6`，y 取根中心
- 主脊 → 一级：`x=436, w=84, h=6`，y 取该枝一级节点中心 − 3
- 一级 → 枝脊：`x=860, w=70, h=4`
- 枝脊：`x=926, w=4`，跨该枝首尾二级节点中心
- 枝脊 → 二级：`x=930, w=50, h=4`，y 取二级节点中心 − 2

二级文本控制在 **44 个汉字以内**（`size: s` 在 560 宽内约两行），否则会溢出 72 px 高度。

**善用颜色表达状态**，别只用来区分分支——例如把「待办 / 缺口」那一枝标红，用户扫一眼
就知道下一步在哪。

---

## 画完必须回读校验

坐标是算出来的，不回读就不知道有没有重叠或空框。优先 `board_describe_room`；
需要看手写/视觉细节时再 `board_screenshot`。不要靠推算下结论。

给 id 用 `shape:<层>-<枝>-<序号>` 这类规则命名，校验和后续 patch 都能靠前缀筛选。

---

## 冷启动与故障排查

### MCP 显示 error / 工具发现失败

`run.mjs` 首次运行会自动 `npm ci --ignore-scripts` 装依赖，装完写就绪标记
`node_modules/.agents-board-mcp-ci`（内容为 `package-lock.json` 的 sha256）。三个判据
任一缺失就会重装：`@modelcontextprotocol/sdk`、`tsx/dist/cli.mjs`、上述标记文件。

手动验证服务器是否能起：

```powershell
$req = '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"probe","version":"1.0"}}}'
$req | mise exec node@20 -- node ".dec/cache/agents-board/skills/agents-board/mcp/run.mjs"
```

正常应返回 `{"result":{...,"serverInfo":{"name":"agents-board","version":"0.3.0"}}...}`。

首次冷启动因为要装依赖会耗时数秒，管道里的请求可能在服务器就绪前就 EOF 了；**装完再跑
第二次**才能拿到响应，这不是故障。

### 已修复：Windows 上 `npm ci` 必然失败（2026-07-27）

`run.mjs` 原先用 `spawnSync('npm', ...)` 且未加 `shell`。Windows 上 `npm` 实为
`npm.cmd`，Node 无法直接执行，会立刻以 `status = null` 失败并被 `?? 1` 兜成退出码 1，
表现为**每次冷启都自杀、且 npm 一行输出都没有**。已加 `shell: process.platform === 'win32'`。

若在别的 Windows 机器上又见到 `npm ci failed with exit 1` 且无 npm 输出，先确认这条
修复是否还在。

### 其它

- 先确认你和用户在**同一个 roomId**，再谈「为什么同步不到」
- 只改图形时优先 `board_patch_shapes`；大块重画再 upsert
- 服务端开发 / 部署：私有仓库 <https://github.com/shichao402/agents-board>（`server/` + `common/`）
- 新工作区只需 clone 该仓库 + `dec pull` `agents-board` bundle，不依赖任何临时目录
