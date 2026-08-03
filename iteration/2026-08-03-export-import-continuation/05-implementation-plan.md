# 实现方案（Implementation Plan）

> 迭代：2026-08 · 状态：待评审 · 关联：《02-prd.md》《04-test-cases.md》

---

## 0. 总览与优先级

| 阶段 | 内容 | 依赖 | 预估 |
|---|---|---|---|
| Phase 1 | 需求一：作品目录导出 | 无 | S |
| Phase 2 | 需求二 + 三：作品目录/书籍设定导入导出 | Phase 1 的打包工具可复用 | M |
| Phase 3 | 需求四：Skills AI 对话页 + 草稿流水线 | 无 | M |
| Phase 4 | 需求五：续写推演（重点） | 无（独立 API 面） | L |

阶段间可并行开发，按序合入。每个 Phase 完成即补对应测试（见《04-test-cases.md》）。

---

## 1. Phase 1：作品目录导出

### 后端
1. **新包 `internal/book/export_pack.go`**（或并入现有 export.go 拆分）：
   - `Service.BuildExportSelection(input)`：按勾选内容组装导出。
   - `zip` 打包：章节 md（保留分卷目录）、设定文件（原相对路径）、`book.json`；单文件勾选直接返回原文。
   - 复用 `ExportText`（txt）；新增 `ExportChaptersMarkdown`（每章 md 组装，含标题头）。
2. **API 扩展**：`GET /api/books/export` 增加参数（`selection` 编码 + `format=txt|md|zip`）；保持旧参数兼容（`format=txt` 行为不变，不破坏 HomeView）。
3. **错误**：无勾选内容 → 明确错误；无导出内容 → 双语错误文案。

### 前端
4. **`web/src/features/chapters/` 或 workbench 下新增 `ExportDialog.tsx`**：内容勾选（章节正文/细纲最新|全部/大纲/规则/进度/角色状态/灵感/元数据）+ 格式联动 + 确认。
5. **入口**：`ModeRouter.tsx` `ChapterOutline` 顶部操作区新增"导出"按钮（复用确认弹窗组件、双语 i18n key）。
6. api-client：`books.ts` 扩展 `exportBook(input: { path; format; selection })`；下载复用 `downloadBookExport`。

### 验证
- U：EXP-001~005；I：EXP-006；M：EXP-007~008。

---

## 2. Phase 2：作品目录导入 + 书籍设定导入导出

### 后端
1. **材料导入识别**：新文件 `internal/book/settings_import.go`：
   - `IdentifySettingsType(filename, content)` → `SettingsImportTarget{Kind, Path}`（标准名匹配 + 标题启发 + 失败返回类型枚举）。
   - `ImportSettingsFile(workspace, file, kind)`：备份（`.casemagica/backups/settings-import/<ts>/`）→ 原子写入；细纲自动编号 `group<N+1>.md`。
2. **API**：
   - `POST /api/books/settings/import`（multipart：file + kind(可选，识别失败时必填)）→ `{target_path, backup_path}`。
   - `GET /api/books/settings/export?file=<相对路径>` → 单文件下载（供书籍设定项导出，含细纲单项）。
   - `GET /api/books/settings/export/zip?group=chapter-groups` → 全部细纲 zip。
   - 导出 API 与 Phase 1 的 zip 打包共用一套服务端逻辑（概念边界：Phase 1 是"作品导出"，Phase 3 是"设定单项导出"，共享 zip 工具）。
3. **复用**：格式导入直接复用现有 `/api/books/import-novel/*`（不改实现）。

### 前端
4. **`SettingsImportDialog.tsx`**：选文件 → 识别摘要（类型→目标路径→备份位置）→ 识别失败时枚举类型手动选择 → 确认。
5. **入口**：ChapterOutline 顶部"导入"按钮（弹窗内两个页签：按格式 / 按材料）。
6. **`BookSettingsShortcuts.tsx` 扩展**：设定项操作菜单加"导出/导入"（大纲、规则、粗细纲）；粗细纲"导出全部"入口。
7. 双语 i18n + 组件复用（Dialog、Select、Checkbox、确认弹窗）。

### 验证
- U：IMP-001~004、SET-001~004；I：IMP-005~006、SET-005；M：IMP-007~008、SET-006~007。

---

## 3. Phase 3：Skills AI 对话页

### 后端
1. **草稿区服务**：`internal/skills/draft.go`：
   - 用户级目录 `.casemagica/skill-drafts/<name>/`（workspacepath 层，注意 `.casemagica` 在用户数据目录 vs 工作区的解析差异——skills 的 user scope 已是用户级，草稿区与之一致）。
   - `CreateDraft(name, files)` / `ListDrafts()` / `ReadDraft(name)` / `DiscardDraft(name)`（丢弃=移动至备份区）/ `ConfirmDraft(name, scope)`（校验 frontmatter → 复用现有 install/创建逻辑 → 删除草稿）。
2. **agent 工具**：`internal/agent/config_manager_tools.go` 或新文件 `skill_draft_tool.go` 增加 `create_skill_draft`（入参：name/description/frontmatter 字段/body/附加文件[]；输出草稿 id + 文件清单）。仅对 config_manager agent 暴露（origin=skills_chat 场景）。
3. **SSE 事件**：chat 事件流新增 `skill_draft` 事件（草稿 id + 摘要 + 文件清单），复用现有事件序列化。
4. **API**：
   - `POST /api/skills/drafts`（创建，供前端直接创建兜底）、`GET /api/skills/drafts`、`GET /api/skills/drafts/:name`、`DELETE /api/skills/drafts/:name`、`POST /api/skills/drafts/:name/confirm`（scope: user|workspace）。

### 前端
5. **SkillsView 增加"AI 对话"页签**（第 5 个模式 `chat`）：复用 `AgentChatPane`/`useAgentChat`（origin=`skills_chat`，展示名"Skills 炼成助手"）。
6. **草稿就绪卡片**：监听 `skill_draft` SSE 事件 → 卡片（名称/描述/文件清单 + 查看/编辑/导入/丢弃）；"查看/编辑"切到 editor 模式加载草稿文档；"导入"调 confirm API 后刷新列表。
7. 复用 SkillCreatePanel/SkillEditor 编辑草稿（草稿模式：保存=更新草稿文件，确认=导入正式）。

### 验证
- U：SKL-001~003；I：SKL-004~005；M：SKL-006~007。

---

## 4. Phase 4：续写推演（重点）

### 后端
1. **新子包 `internal/continuation/`**（职责单一：推演任务生命周期 + 候选生成 + 提交）：
   - `task.go`：任务注册表（工作区级并发限制）、任务状态机（pending/running/partially_done/done/cancelled/failed）、落盘 `.casemagica/continuations/<task-id>/`。
   - `context.go`：有界上下文组装（复用 `book.State` 的 stable/dynamic parts + `read_file` 窗口读取，见 PRD 5.5）。
   - `generate.go`：逐候选独立 LLM 调用（小并发 2 路，`errgroup` 或手写并发 + recover）；每个候选结构化 prompt（方向级/片段级 × 短/中/长）；输出解析走 json_fallback 兼容。
   - `commit.go`：提交写入（新建章节命名 / 追加章末），走 workspacechange 原子写 + revision 校验。
2. **API**：
   - `POST /api/continuation/explore`（SSE 流式；入参：workspace、anchor_chapter_path、depth、mode、style_refs、preference）→ 事件 `candidate_start / candidate_delta / candidate_done / explore_done / explore_error / explore_cancelled`。
   - `GET /api/continuation/tasks?workspace=`（历史任务列表）、`GET /api/continuation/tasks/:id/stream`（重连）、`POST /api/continuation/tasks/:id/abort`、`DELETE /api/continuation/tasks/:id`（删除→备份）。
   - `POST /api/continuation/commit`（candidate 编辑后内容 + target: new_chapter|append_current）。
3. **配置项**：`continuation.max_candidates/excerpt_max_chars/prefix_chars/timeout_minutes`（config.toml + 默认值，遵循"不写死超时"约定，0=不限）。

### 前端
4. **`web/src/features/chapters/continuation/` 子目录**：
   - `ContinuationDialog.tsx`（配置表单：锚点章节/深度/粒度联动/风格模板选择/方向偏好 + 配置持久化 localStorage）。
   - `ContinuationPanel.tsx`（候选卡片列表：状态/标题/摘要/片段编辑/选择/提交/中止/重新推演；历史推演列表）。
   - 提交目标选择（新建章节 / 追加章末）与编辑器脏状态检查（复用编辑器 dirty 信号）。
5. **触发入口三处**：编辑器工具栏按钮（锚点=当前文件）、章节树章节项操作菜单、命令面板。
6. **api-client**：`continuation.ts`（SSE 客户端，复用 parseSSEStream/AgentChatTransport 模式）。

### 验证
- U：CON-001~008；I：CON-009~011；M：CON-012~018。

---

## 5. 工程事项（贯穿）

1. **双语**：所有新 UI 文案同时添加 zh-CN / en-US i18n key（`web/src/i18n/locales/`）。
2. **日志**：新后端逻辑按项目约定打结构化日志（含文件:行号定位）。
3. **配置**：新增配置项集中在 `config/config.go` 默认值表 + `config.toml` 注释说明（PRD 5.7）。
4. **CHANGELOG**：每阶段合入时更新 `CHANGELOG.md`（中英双语，说明 beta 兼容性影响）。
5. **代码规范**：新文件按职责拆分（不堆大文件）；goroutine 均 recover；早返回；穷尽状态机（任务状态/导入类型/导出勾选）。
6. **测试纪律**：修复 bug 先写复现测试；新功能补对应 U/I 测试；单测 ≤1s。

## 6. 交付顺序

Phase 1 → Phase 2 → Phase 3 → Phase 4；每个 Phase 完成 → 自测（对应测试用例）→ 更新 CHANGELOG → 提交（英文 commit message）。
