# 迭代更新日志（2026-08-03 · Export / Import / Skills Chat / Continuation）

> 迭代目录：`iteration/2026-08-03-export-import-continuation/`
> 提交：`7bc9de6`（分支 `feature/client_pack`）· 当前基线：v0.3.0
> 本迭代变更随下个正式 Release 发布（当前位于 CHANGELOG.md `[Unreleased]`）。

## 迭代概要 / Summary

本轮迭代为作品管理、Skills 与写作流程新增四项能力：作品目录导入导出、书籍设定单文件导入导出、Skills AI 对话炼成流水线，以及基于当前章节的续写可能性推演（重点）。

This iteration adds four capabilities around book management, Skills, and the writing flow: Works Catalog export/import, per-setting-file export/import, a Skills AI Chat crafting pipeline, and continuation exploration from the current chapter (the focus area).

## Added / 新增

- **作品目录导出**：按勾选内容导出（章节正文保留分卷目录、章节组细纲最新或全部、大纲、规则、进度、角色状态、灵感、书籍元信息），格式支持 txt / md / zip；浏览器默认下载。
- **Works Catalog export**: selectable content (chapter text with volume directories, group outlines latest or all, outline, rules, progress, character states, ideas, book metadata) in txt / md / zip; downloads via the browser default directory.
- **作品目录导入**：按格式复用小说分章建书；按材料自动识别设定文件写入对应位置（识别失败由用户手动选择目标类型，写入前自动备份，可回滚）。
- **Works Catalog import**: format-based novel chapter splitting for new books; material-based setting files written to their matching locations with automatic identification (manual fallback on failure), automatic backup before writing.
- **书籍设定导入导出**：大纲（setting/outline.md）、规则（CREATOR.md）、章节组细纲（setting/chapter-groups/）可独立导出/导入；细纲支持单个或全部打包，导入自动编号不覆盖。
- **Book settings export/import**: outline, rules, and chapter group outlines export/import individually; group outlines export singly or as a zip; imports are auto-numbered without overwrites.
- **Skills AI 对话页**：与 Config Manager 对话炼成 Skill；Agent 经 `create_skill_draft` 工具产出用户级草稿（`<dataDir>/skill-drafts/`），对话页可预览、编辑、确认导入（user/workspace）或丢弃（移入备份区）。
- **Skills AI Chat page**: craft Skills by chatting with the Config Manager; the Agent produces user-scoped drafts via `create_skill_draft`, previewable, editable, confirmable (user/workspace) or discardable (moved to backups).
- **续写推演**：方向级/片段级粒度 × 短/中/长三档深度，可配置锚点章节、风格模板（复用 `.casemagica/styles` 文风参考）与方向偏好；候选逐个生成（SSE，部分失败不影响其他），可编辑后提交为新章节（`ch%05d-标题.md`）或追加章末，写入走 workspacechange 原子变更（可审阅、可撤销）；任务落盘 `.casemagica/continuations/<task>/`，刷新可恢复，删除前自动备份。
- **Continuation exploration**: direction/excerpt granularity × short/medium/long depth, with anchor chapter, style template (reusing `.casemagica/styles` references) and direction preference; candidates stream in one by one (partial failure tolerated), editable and commitable as a new chapter or an append to the anchor chapter through atomic reviewable changes; tasks persist under `.casemagica/continuations/<task>/` and survive refreshes, with backups before deletion.
- **品牌图标重设计**：左上角品牌图标、浏览器 favicon 与 PWA 图标统一为极简风格（深色圆角方块 + 白色打开书线条 + 红黄绿星光），移除原有渐变与光晕滤镜。
- **Brand icon redesign**: top-left brand icon, browser favicon, and PWA icons unified in a minimal style (dark rounded square + white open-book outline + red/yellow/green sparkle dots), removing gradients and glow filters.

## Changed / 变更

- `GET /api/books/export` 扩展内容勾选参数与 md/zip 格式；不带勾选参数时保持旧行为（向后兼容）。
- `GET /api/books/export` now supports content-selection params and md/zip formats; legacy behavior is preserved without them.
- Skills 页顶部入口改为两行两列布局；AI 对话与配置面板互斥（激活一方时另一方置灰），避免同时出现两个对话面板。
- The Skills page top entries use a two-by-two layout; AI Chat and the config panel are mutually exclusive to prevent two chat panels at once.
- 新增配置 `[continuation]`：`max_candidates=3`、`excerpt_max_chars=1500`、`prefix_chars=3000`、`timeout_minutes=0`（0=不限制）。
- New `[continuation]` config section with the defaults above (timeout 0 = unlimited).

## API 变更 / API changes

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/books/export` | 扩展勾选参数 + md/zip |
| POST | `/api/books/settings/identify` | 材料识别预览（可指定 kind） |
| POST | `/api/books/settings/import` | 材料导入（自动备份） |
| GET | `/api/books/settings/export` / `export/groups` | 设定单文件 / 全部细纲导出 |
| POST/GET/PUT/DELETE | `/api/skills/drafts[/:name][/confirm]` | Skill 草稿 CRUD 与确认导入 |
| POST | `/api/continuation/explore` | 发起推演（SSE） |
| GET | `/api/continuation/tasks` / `tasks/:id/stream` | 任务列表 / 恢复快照 |
| POST/DELETE | `/api/continuation/tasks/:id/abort` / `tasks/:id` | 中止 / 删除（备份） |
| POST | `/api/continuation/commit` | 提交候选（新建章节 / 追加章末） |

## 兼容性说明 / Compatibility

- Beta 迭代，无既有协议破坏；导出旧参数行为不变。
- 新增写路径全部具备备份或可撤销能力（材料导入 → `backups/settings-import/`；推演提交 → workspacechange 账本；草稿/任务删除 → 备份区）。
- 新增能力均为写作模式侧；游戏模式未改动，两个模式可独立使用。
- No breaking changes; all new write paths are backed up or undoable. New features are writing-mode scoped; Game Mode is untouched.

## 测试与验证 / Testing

- 后端 `go test ./...`：36 个包全绿（含新增 export_pack / settings_import / skill drafts / continuation 单测与 API 集成测试）。
- 前端 `tsc` 无错误，Vitest 124 文件 / 645 用例全绿；`./scripts/build.sh` 生产构建通过。
- 真机冒烟：真实 LLM 短推演（候选生成、落盘、提交新建章节与追加章末、删除备份）全部通过；材料导入识别/备份/导出往返通过。
- 浏览器手动验证清单见《08-test-regression.md》§5（待用户执行）。

## 已知限制 / Known limitations

- 推演中刷新页面的实时重连为快照恢复（已完成候选可见），进行中的生成不续流。
- 推演候选生成依赖模型 JSON 输出，解析失败时该候选标记失败（不影响其余候选）。
