# 迭代需求梳理（原始需求 + 现状分析）

> 迭代周期：2026-08 · 状态：评审中
> 对应分支：`feature/client_pack` · 当前版本：v0.3.0

## 一、原始需求（用户原话整理）

1. **作品目录导出**：在页面上"作品目录"选项卡下新增导出按钮，导出能力，支持选择导出到指定目录。
2. **作品目录导入**：在页面上"作品目录"选项卡下新增导入按钮，导入能力，支持按格式和原始材料导入。
3. **书籍设定导入导出**：在"作品目录-书籍设定"里，新增针对大纲、规则、粗细纲的导出、导入能力。
4. **Skills AI 对话页**：在 skills 菜单里，新增 AI 对话页，辅助进行 skills 炼成，支持自动化 skills 生成并导入流水线。
5. **续写可能性推演**（重点评估细化）：期望新增一个能力，基于当前书籍章节，进行续写可能性的推演供用户选择/修改/提交。

## 二、现状分析（代码调研结论）

### 2.1 页面结构（需求 1、2、3 的落点）

- 一级菜单（WorkbenchShell.tsx:59）：写作 writing / 故事 story / 时间线 timeline / 资料 lore / 讲述者 teller / 作品 books / 版本 versions / 技能 skills / 智能体 agents / 自动化 automations。
- "作品目录"是**写作模式侧边栏左侧的三个视图按钮之一**（作品目录 / 项目文件 / 全局搜索，ModeRouter.tsx:435-457），选中后渲染 `ChapterOutline` 组件（ModeRouter.tsx:834-987），内容包括：
  1. `BookSettingsShortcuts`（书籍设定收藏区，workbench/BookSettingsShortcuts.tsx）
  2. 章节组细纲列表（最新 + 历史，来自 `setting/chapter-groups/*.md`）
  3. 分卷章节树（volumes/chapters，来自 `chapters/**/*.md`，含字数/状态/确认）
- **书籍设定文件清单**（均为 .md）：

| 内容项 | 路径 | 说明 |
|---|---|---|
| 大纲 | `setting/outline.md` | 长期大纲（book/summary.go:84） |
| 规则 | `CREATOR.md`（workspace 根） | 创作者规则（book/creator.go:12） |
| 进度 | `setting/progress.md` | 写作进度（state.go:184） |
| 角色状态 | `setting/character-states.md` | 角色状态（state.go:79） |
| 灵感 | `ideas.md`（根） | 创作灵感（state.go:71） |
| 粗细纲 | `setting/chapter-groups/*.md` | 章节组细纲（state.go:66-68） |
| 元数据 | `book.json` | 书名/作者/简介（state.go:454-497） |

### 2.2 已有导入导出能力（需求 1、2、3 的复用基础）

**导出：**
- `GET /api/books/export`（routes.go:56 → handler_books.go:125）导出**全书纯文本 txt**（仅章节正文，`internal/book/export.go`），入口在 **HomeView（作品管理首页）**，不在作品目录侧栏（HomeView.tsx:170-184、393-400）。
- 前端下载：`downloadBookExport`（lib/api-client/books.ts:150-159），Blob + `<a download>`，**无目录选择器**（无 showDirectoryPicker/webkitdirectory 先例）。
- 大纲/规则/粗细纲**没有**独立的导出导入能力；版本管理是整工作区级 restore，非单文件。

**导入：**
- `POST /api/books/import-novel/preview` / `/preview/stream` / `/api/books/import-novel`（routes.go:57-59），支持 txt/md、UTF-8/UTF-16/GB18030 解码、4 种分章策略（builtin / local_regex / tool_agent_regex / custom_regex）、预览 + 分章（novel_import.go:140-181），入口同样在 HomeView 的 `NovelImportDialog`。
- 角色卡导入：`/api/workspace/import-character-card`（PNG/JSON）。
- Skills 导入：zip / GitHub / 远程 URL 两段式（preview + install）。

### 2.3 Skills 能力（需求 4 的落点）

- Skills 是**一级共享菜单**（mode === 'skills'，WorkbenchShell.tsx:177-184、ModeRouter.tsx:647 → `SkillsView`）。
- `SkillsView`（features/skills/SkillsView.tsx）已有四个模式：`editor | create | config | install`，并**内嵌 ConfigManagerChat agent 面板**（origin="skills"，L318-331）；SkillCreatePanel 已有 "Ask Agent" 按钮（L129-138）。
- 后端 skills API：管理面（`GET /api/skills`、文档/文件读写、创建、删除、恢复内置）+ 安装面（`/api/skills/install/{zip|remote|github}/preview|install`）。
- **已有 `skills-creator` 内置 skill**（skills/skills-creator/SKILL.md，prompt 型）：指导 agent 创建/修改/审查用户或工作区 scope 的 skills。
- Skill 结构：`<scope>/<name>/SKILL.md`，frontmatter 含 name/description/agent/context/model；三个 scope：builtin（只读）/ user（`.casemagica/skills`）/ workspace（`<workspace>/skills`）。
- 本地目录安装 `PreviewDirectory`/`InstallFromDirectory` 后端已有但**未暴露 HTTP 路由、前端未接**。

### 2.4 续写能力（需求 5 的现状）

- **无专用后端工具/API**。续写靠三件套：
  1. system prompt 内建续写工作流（prompts/system.go:204 的 8 步）；
  2. `continue` skill（prompts 型，agent: ide, interactive_story）；
  3. 前端 `/continue` 快捷消息（App.tsx:597-599，命令面板 ⌘↵）+ automation `continue_writing` 模板（默认关闭）。
- 上下文支撑：`book.State` 自动注入 outline / progress / character-states / chapter-groups / chapter_paths（有界），正文靠 `read_file` 有界读取（workspace_read_file.go:56，单窗口 ≤1MB）。
- **没有"多方向推演 → 用户选择/修改/提交"的交互**，用户无法在生成前预览多个续写方向，也无法定向编辑后落盘。

### 2.5 前端交互基础

- 文件选择均为 `<input type="file">`（无目录选择器先例）；下载为 Blob + `<a download>`。
- 流式聊天客户端现成：`useAgentChat` hook（hooks/useAgentChat.ts:56）+ `AgentChatTransport`（lib/agent-ui.ts:67，SSE，支持重连）。
- 命令面板（common/command-palette.tsx）、确认弹窗、双语 i18n、shadcn/ui 组件库均齐备。

## 三、需求初步理解与待确认项

### 3.1 需求 1（作品目录导出）

- 位置：写作模式侧边栏"作品目录"视图（ChapterOutline）顶部新增导出入口（按钮/图标）。
- 能力：导出作品内容（章节全文），**支持选择导出到指定目录**。
- 歧义点 A：导出物范围——仅章节正文（txt）还是包含书籍设定的"完整作品包"（章节 + 大纲 + 规则 + 细纲）？
- 歧义点 B："指定目录"的交互——浏览器端 `showDirectoryPicker`（Chrome 系，Safari/Firefox 需降级浏览器默认下载）还是服务端指定路径？

### 3.2 需求 2（作品目录导入）

- 位置：同上视图新增导入入口。
- 能力：**按格式和原始材料导入**。
- 歧义点 C："按格式"是否指复用现有小说导入（txt/md 自动分章）？
- 歧义点 D："按原始材料"指什么——书籍设定文件（大纲/规则/细纲 md）、资料库材料、还是任意素材文件归档？

### 3.3 需求 3（书籍设定导入导出）

- 位置：书籍设定（BookSettingsShortcuts 收藏区）内针对**大纲、规则、粗细纲**三个文件类型提供导入/导出。
- 能力：单文件导出（下载或指定目录）、单文件导入（选择文件后写入对应设定文件，保留备份/可回滚）。
- 待确认 E：导出文件命名与格式（md 原文直出？带 frontmatter 的封装包？）。

### 3.4 需求 4（Skills AI 对话页）

- 位置：Skills 菜单下新增独立"AI 对话"页（区别于当前内嵌的小面板）。
- 能力：与 agent 对话炼成 skill；**自动化生成并导入流水线**——描述需求 → agent 生成 SKILL.md（+附加文件）→ 预览 → 导入到 user/workspace scope → 立即可用。
- 复用：现有 chat 链路 + createSkill/install API + skills-creator skill + ConfigManagerChat 组件。

### 3.5 需求 5（续写推演，重点评估细化）

- 位置：写作模式，基于当前章节（编辑器内或章节树/命令面板触发）。
- 期望形态：基于当前书籍上下文推演 **N 个续写可能性** → 用户**选择/修改** → **提交**（落盘到章节）。
- 待确认 F（核心设计分叉）：
  - 推演粒度：a) 方向级（每个候选 = 剧情方向摘要 + 标题建议，选定后再生成正文）；b) 片段级（每个候选直接含一段可编辑正文开头）；c) 两级合并（方向 + 正文片段一次性产出）。
  - 提交落点：a) 追加当前章节末尾；b) 新建下一章（自动命名 ch0000N-标题.md）；c) 用户自选。
  - 候选数量与并发：默认 N=3？并行生成的成本与超时约束。
  - 与现有 `/continue` 的关系：保留现有能力，新功能作为显式交互入口。

## 四、评估结论（概要）

| 需求 | 工作量估计 | 主要复用 | 主要新工作 |
|---|---|---|---|
| 1 作品目录导出 | S | export.go、downloadBookExport | 入口迁移/新增、目录选择交互、导出范围扩展 |
| 2 作品目录导入 | M | novel_import.go、NovelImportDialog | 入口迁移/新增、材料导入解析与落盘策略 |
| 3 书籍设定导入导出 | S | files.go 读写、版本恢复链路 | 单文件导出/导入 API、备份回滚 |
| 4 Skills AI 对话页 | M | chat 链路、skills API、skills-creator | 独立对话页、生成→预览→导入流水线 |
| 5 续写推演 | L（重点） | book.State 上下文、read_file、workspacechange | 多候选推演 API（并行 LLM）、候选选择/编辑 UI、提交落盘协议 |

整体可行：需求 1-4 均有成熟复用基础；需求 5 是本轮迭代的核心增量，需重点评审设计（见 PRD 与技术方案章节）。
