# 技术方案（Technical Design）

> 迭代：2026-08 · 状态：待评审 · 关联：《02-prd.md》《05-implementation-plan.md》

---

## 1. 架构总览

```
┌─ web/src ──────────────────────────────────────────────┐
│ features/workbench: ExportDialog / ImportDialog         │
│ features/skills: SkillsView + ChatTab + DraftCards      │
│ features/chapters/continuation: 推演配置/候选面板/提交   │
└──────────────┬─────────────────────────────────────────┘
               │ REST + SSE
┌──────────────▼─────────────────────────────────────────┐
│ internal/api (routes/handlers)                          │
│  ├─ books export/import（Phase1/2）                     │
│  ├─ skills drafts（Phase3）                             │
│  └─ continuation explore/tasks/commit（Phase4）         │
└──────────────┬─────────────────────────────────────────┘
┌──────────────▼─────────────────────────────────────────┐
│ 业务层                                                     │
│  ├─ internal/book: export_pack / settings_import        │
│  ├─ internal/skills: draft.go（草稿区）                   │
│  └─ internal/continuation: task/context/generate/commit │
└──────────────┬─────────────────────────────────────────┘
┌──────────────▼─────────────────────────────────────────┐
│ 基础能力（复用，不重造）                                  │
│  ├─ internal/workspacechange: 原子写 + revision + 账本   │
│  ├─ internal/book (State): 上下文片段/章节枚举/文件服务    │
│  ├─ internal/agent (runner/model/json_fallback): LLM 调用 │
│  └─ internal/workspacepath: .casemagica 路径解析          │
└────────────────────────────────────────────────────────┘
```

设计原则：
- **推演不进入现有 chat/agent 运行管道**（它是一次性并行生成，非多轮对话），新建独立 `internal/continuation` 子包，仅复用底层 LLM 调用与 workspacechange 写入。
- **Skill 草稿进入 chat 管道**（config_manager agent 通过工具产出），复用现有事件流在对话内通知前端。

---

## 2. 后端模块设计

### 2.1 `internal/book/export_pack.go`（Phase 1/2 共享）

```go
type ExportSelection struct {
    Chapters         bool   // 章节正文（md/zip）
    ChapterGroups    string // "" | "latest" | "all"（细纲）
    Outline, Rules, Progress, CharacterStates, Ideas, Meta bool
    // 仅支持单个文件勾选时，直接返回原文而非 zip
}

type ExportPack struct {
    Format  string          // txt | md | zip
    Files   []ExportFile    // zip 内容（相对路径 + 内容）
    Content string          // txt/md 单文件模式
    Name    string          // 下载文件名
}

func (s *Service) BuildExportSelection(sel ExportSelection) (ExportPack, error)
```

- zip 用标准库 `archive/zip`（项目无 zip 依赖；skills install 已用 archive/zip）。
- 章节 md 组装：读取章节文件原文，文件内若已有标题头则保留原样（不重复加标题）；目录结构 `chapters/<分卷>/<章节文件>` 按 `book.Summary().Chapters` 相对路径。
- 旧 `GET /api/books/export?format=txt` 保持兼容：无 selection 参数时行为不变。

### 2.2 `internal/book/settings_import.go`（Phase 2）

```go
type SettingsKind int
const (
    SettingsKindUnknown SettingsKind = iota
    SettingsKindOutline      // setting/outline.md
    SettingsKindRules        // CREATOR.md
    SettingsKindProgress     // setting/progress.md
    SettingsKindCharacterStates // setting/character-states.md
    SettingsKindIdeas        // ideas.md
    SettingsKindChapterGroup // setting/chapter-groups/group<N>.md
)

func IdentifySettingsKind(filename, content string) SettingsKind
func (s *Service) ImportSettingsFile(src []byte, kind SettingsKind) (targetPath, backupPath string, err error)
```

- 识别：文件名优先（`outline.md`/`CREATOR.md`/`progress.md`/`character-states.md`/`ideas.md`），其次内容启发（标题含"大纲/规则/细纲/进度/角色"等），失败返回 `SettingsKindUnknown`。
- 备份：`.casemagica/backups/settings-import/<yyyyMMdd-HHmmss>/` 下按原相对路径保存；细纲目标 `group<N+1>.md`（枚举现有 `setting/chapter-groups/`）。
- 写入：直接写文件系统原子写（`os.WriteFile` 到临时文件再 rename），不走 workspacechange（用户级导入是非 Agent 编辑，无需进入变更审阅账本；但记录日志）。**评审点：是否应走账本？** 见 §7 R-6。

### 2.3 `internal/skills/draft.go`（Phase 3）

用户级草稿区：`<denovaDir>/skill-drafts/<name>/`（与 user scope skills 同级，跨工作区共享）。

```go
type DraftMeta struct {
    Name        string    `json:"name"`
    Description string    `json:"description"`
    SourceScope string    `json:"source_scope"` // 来源工作区
    CreatedAt   time.Time `json:"created_at"`
    Files       []string  `json:"files"`
}

func CreateDraft(name string, files map[string][]byte, meta DraftMeta) (id string, err error)
func ListDrafts() ([]DraftMeta, error)
func ReadDraft(name string) (map[string][]byte, error)
func DiscardDraft(name string) (backupPath string, err error) // 移至 backups/skill-drafts/
func ConfirmDraft(name string, scope string) (err error)
```

- `ConfirmDraft`：读草稿 → `parseFrontmatter` 校验（name 合法字符、description 必填）→ 写入目标 scope（user=`.casemagica/skills/`、workspace=`<ws>/skills/`）→ 删除草稿；同名冲突返回明确错误由前端提示覆盖/改名。
- 安全：name 必须通过 `skillNamePattern`（复用 skills 包既有校验），禁止路径穿越。

### 2.4 `internal/agent/skill_draft_tool.go`（Phase 3）

新增 config_manager 工具 `create_skill_draft`：

```go
type createSkillDraftInput struct {
    Name        string            `json:"name" jsonschema:"required,..."`
    Description string            `json:"description" jsonschema:"required,..."`
    Agent       string            `json:"agent,omitempty"`
    Context     string            `json:"context,omitempty"`
    Model       string            `json:"model,omitempty"`
    Body        string            `json:"body" jsonschema:"required,..."`
    Files       map[string]string `json:"files,omitempty"` // 附加文件路径->内容
}
```

- 输出：`{draft_id, name, files: [...]}`。
- 触发方式：agent 调用工具后，运行管道在工具结果旁发 `skill_draft` 事件（Data：草稿摘要）。实现：工具执行后由 `builder.go` 的 config_manager 工具列表注册时带一个 hook，或在工具函数内直接调用 `s.emit` 不可行（工具无 emit 引用）→ **采用方案**：`create_skill_draft` 工具本身返回结构化结果，前端从 tool_result 中识别 `skill_draft` 型结果并渲染卡片（与 `workspace_change` 事件卡片同模式，chat_tool_log.go 现有工具结果展示链路可扩展）。见 §7 R-7。
- 后端另提供 REST `POST /api/skills/drafts` 供前端"直接创建草稿"兜底与测试。

### 2.5 `internal/continuation/`（Phase 4，新子包）

文件划分（每个文件职责单一，避免大文件）：

| 文件 | 职责 |
|---|---|
| `types.go` | 枚举（Depth: short/medium/long、Mode: direction/excerpt、Target: new_chapter/append）、请求/候选/任务结构体 |
| `task.go` | 任务注册表（sync.Map，key=workspace 级互斥）、状态机、落盘目录结构 |
| `context.go` | 有界上下文组装（复用 `book.State` stable/dynamic parts + 章节窗口读取） |
| `generate.go` | 逐候选 LLM 调用（小并发 2 路 + recover + 部分失败容忍）、prompt 组装、输出解析（json_fallback） |
| `commit.go` | 提交写入：新建章节命名 / 追加章末，走 workspacechange |
| `store.go` | 落盘读写（request.json / candidates/<i>.json / meta.json） |

```go
type ExploreRequest struct {
    Workspace        string   `json:"workspace"`
    AnchorChapter    string   `json:"anchor_chapter_path,omitempty"` // 默认最近章节
    Depth            string   `json:"depth"`      // short|medium|long
    Mode             string   `json:"mode"`       // direction|excerpt
    StyleRefNames    []string `json:"style_ref_names,omitempty"` // .casemagica/styles 已有参考
    CustomStyle      string   `json:"custom_style,omitempty"`
    Preference       string   `json:"preference,omitempty"` // 方向偏好自由文本
}

type Candidate struct {
    Index     int    `json:"index"`
    Title     string `json:"title"`
    Direction string `json:"direction"` // 方向摘要
    Excerpt   string `json:"excerpt"`   // 片段级正文开头
    Plan      string `json:"plan"`      // 长推演的后续走向规划
    Status    string `json:"status"`    // generating|done|failed
    Error     string `json:"error,omitempty"`
}

type CommitRequest struct {
    TaskID  string   `json:"task_id"`
    Index   int      `json:"index"`
    Title   string   `json:"title"`     // 用户编辑后
    Content string   `json:"content"`   // 用户编辑后的正文（片段级）或方向说明（方向级新建章节时的占位正文）
    Target  string   `json:"target"`    // new_chapter | append
}
```

**状态机（穷尽）**：

```
pending → running → done
                  → partially_done（部分候选失败）
                  → cancelled（用户中止）
                  → failed（全部失败）
running/cancelled 间可 abort；done/partially_done/failed 可 delete（→备份）
```

**任务落盘**（PRD 5.6）：

```
<workspace>/.casemagica/continuations/<task-id>/
  meta.json        # 请求 + 状态 + 时间戳
  candidates/0.json
  candidates/1.json
  ...
```

**生成并发模型**：
- 候选数 n = min(请求数, config.continuation.max_candidates)，深度映射默认：short=2、medium=3、long=3。
- `generate.go` 用带缓冲 channel 的 worker 模式，并发度 2；每个候选 goroutine 内 `defer recover`；单个失败 → 该候选 status=failed + error，其余继续；全部失败 → 任务 failed。
- LLM 调用复用 `agent` 包的模型构造（`agent.ChatModelFor(...)` 类入口，确认具体工厂函数后接线）；prompt 用 `schema.UserMessage` + system，无工具（纯生成）。输出要求 JSON（方向级/片段级结构不同），解析失败走 `agent.json_fallback` 兼容。
- 中止：任务持有 `context.CancelFunc`，abort API 触发取消；生成循环检查 ctx.Err。

**SSE 事件协议**（`POST /api/continuation/explore` 与 `GET /api/continuation/tasks/:id/stream` 共用）：

```jsonc
{"type":"candidate_start",   "task_id":"...", "index":0}
{"type":"candidate_delta",   "task_id":"...", "index":0, "content":"片段流式增量"}
{"type":"candidate_done",    "task_id":"...", "index":0, "candidate":{...}}
{"type":"candidate_failed",  "task_id":"...", "index":0, "error":"..."}
{"type":"explore_done",      "task_id":"...", "status":"done|partially_done"}
{"type":"explore_cancelled", "task_id":"...", "status":"cancelled"}
{"type":"explore_error",     "task_id":"...", "error":"..."}
```

事件格式对齐现有 `agent.Event{Type, Data}` 与 `writeEvent`（internal/api/sse/task.go:169）。

**提交写入（commit.go）**：
- `new_chapter`：复用 novel_import 的章节命名逻辑（`ch%05d-%s.md`，取 `chapters/` 现有最大编号 +1；标题清洗非法字符），写入 `chapters/<默认卷>`（无分卷则 `chapters/` 根）。走 `workspacechange.Service.ApplyEdits`（新建文件用 ReplaceFile）。
- `append`：`ApplyEdits` 对锚点章节追加文本（BaseRevision 为当前文件 revision；前端传编辑器未保存则拒绝，见 PRD 5.8/评审 P5-15）。
- 提交结果事件进 Change Review 链路（workspacechange 账本天然支持）。

### 2.6 API 路由汇总（internal/api/routes.go 追加）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/books/export` | 扩展 selection 参数（Phase1） |
| POST | `/api/books/settings/import` | 材料导入（multipart） |
| GET | `/api/books/settings/export` | 设定单项导出（?path=） |
| GET | `/api/books/settings/export/groups` | 全部细纲 zip |
| POST | `/api/skills/drafts` | 创建草稿 |
| GET | `/api/skills/drafts` | 列表 |
| GET | `/api/skills/drafts/:name` | 读取草稿 |
| DELETE | `/api/skills/drafts/:name` | 丢弃（→备份） |
| POST | `/api/skills/drafts/:name/confirm` | 确认导入（?scope=user\|workspace） |
| POST | `/api/continuation/explore` | 推演（SSE） |
| GET | `/api/continuation/tasks` | 历史列表 |
| GET | `/api/continuation/tasks/:id/stream` | 重连 |
| POST | `/api/continuation/tasks/:id/abort` | 中止 |
| DELETE | `/api/continuation/tasks/:id` | 删除（→备份） |
| POST | `/api/continuation/commit` | 提交候选 |

### 2.7 配置项（config/config.go + config.toml）

```toml
[continuation]
max_candidates = 3        # 1~5，单次推演最大候选数
excerpt_max_chars = 1500  # 长推演片段上限（字）
prefix_chars = 3000       # 单章前文注入上限（字）
timeout_minutes = 0       # 0=不限（默认）；可选超时
```

---

## 3. 前端设计

### 3.1 作品目录导出/导入（Phase 1/2）

- 新组件 `web/src/features/workbench/ExportDialog.tsx`、`SettingsImportDialog.tsx`（复用 shadcn Dialog/Checkbox/Select/RadioGroup 与既有确认弹窗）。
- `ChapterOutline` 顶部新增操作区：`[导入] [导出]` 按钮（i18n：`workspace.import`/`workspace.export` 等）。
- `books.ts` api-client 扩展 `exportBook`（selection 参数）+ `settingsImport` + `settingsExportFile` + `settingsExportGroups`。
- `BookSettingsShortcuts.tsx`：设定项行加操作菜单（导出/导入），粗纲项额外"导出全部"；进度/角色状态/灵感不提供独立按钮（PRD 3.3）。

### 3.2 Skills AI 对话页（Phase 3）

- `SkillsView` 模式扩展：`'editor' | 'create' | 'config' | 'install' | 'chat'`；顶部页签新增"AI 对话"。
- 新组件 `web/src/features/skills/SkillChatTab.tsx`：
  - 复用 `AgentChatPane`（origin=`skills_chat`，agentKind=config_manager）；
  - 草稿卡片：从工具结果（`create_skill_draft` 返回值）渲染"Skill 草稿就绪"卡片（名称/描述/文件清单/时间）；
  - 操作：查看/编辑（切到 editor 模式，草稿作为文档加载）→ 保存草稿（PUT drafts）→ 确认导入（POST confirm）→ 丢弃（DELETE）。
- 草稿列表入口：对话页侧边小列表（GET /api/skills/drafts），可恢复未处理的草稿。

### 3.3 续写推演（Phase 4）

- 新目录 `web/src/features/chapters/continuation/`：
  - `ContinuationDialog.tsx`：配置表单（锚点章节/深度/粒度联动/风格模板下拉（GET /api/styles）/自定义文风/方向偏好）+ 配置 localStorage 持久化 + 历史推演列表。
  - `ContinuationPanel.tsx`：候选卡片（状态徽标/标题可编辑/摘要可编辑/片段 Markdown 可编辑/展开全文/失败标识）+ 操作区（选择、重新推演、全部丢弃、中止）+ 提交弹窗（目标选择：新建章节|追加章末）。
  - SSE 客户端 `continuation.ts`：解析候选事件流；断线后按 task_id 重连 `tasks/:id/stream`。
- 触发：编辑器工具栏按钮（App.tsx/Editor 工具栏扩展，锚点=当前打开章节）、章节树章节项菜单（ModeRouter ChapterOutline 项操作）、命令面板（`command-palette.tsx` 新增 id=`continuation`）。
- 编辑器脏状态检查：提交前读取编辑器保存状态（现有 dirty 机制），脏则提示保存。

---

## 4. 数据与文件布局

| 数据 | 位置 | 生命周期 |
|---|---|---|
| 导出 zip | 内存组装，无落盘 | 请求级 |
| 材料导入备份 | `<ws>/.casemagica/backups/settings-import/<ts>/` | 保留（清理策略沿用既有 backups 约定） |
| Skill 草稿 | `<denovaDir>/skill-drafts/<name>/`（用户级） | 确认导入/丢弃后删除；丢弃移备份区 |
| 推演任务 | `<ws>/.casemagica/continuations/<task-id>/` | 用户删除或保留；删除移备份区 |

路径解析统一走 `internal/workspacepath`（兼容 `.denova` 旧名）。

## 5. 安全与边界

- Skill 草稿 name 严格校验（字符集 + 长度 + 路径穿越拒绝）；zip 解包不涉及（本迭代无 zip 导入新增面）。
- 材料导入限制扩展名 `.md`（大小上限复用现有上传限制常量，如 64MB 对齐 novel_import）。
- 推演 API 的锚点路径必须经 `book` 章节枚举校验，拒绝任意路径。
- 推演无 LLM 工具调用面（纯生成），避免 Agent 文件写权限扩散；提交走 workspacechange 单一写面。
- 所有并发 goroutine 加 recover（AGENTS.md）。

## 6. 测试映射（与《04-test-cases.md》对应）

- U：EXP-001~005 / IMP-001~004 / SET-001~004 / SKL-001~003 / CON-001~008 → 各模块单测（新文件同目录 `_test.go`）。
- I：EXP-006 / IMP-005~006 / SET-005 / SKL-004~005 / CON-009~011 → API 集成测试（复用 `internal/api/handlers` 现有测试脚手架）。
- M：剩余用例 → 实现完成后手动回归（《08-test-regression.md》记录）。

## 7. 待评审问题

| # | 问题 | 建议 |
|---|---|---|
| R-1 | 推演的 LLM 调用工厂：确认 `agent` 包对外暴露的模型构造入口（现有 `GenerateAutomationTriggerEvaluation` 等模式的 `agent.ChatModelFor` 类函数） | 复用同类入口，不新增底层调用 |
| R-2 | `create_skill_draft` 工具结果如何通知前端：方案 A=工具返回结构化结果，前端识别渲染卡片；方案 B=事件流新事件 | 建议 A（最小改动，复用工具结果链路） |
| R-3 | 方向级候选提交为新建章节时的正文内容：无片段时正文写什么？ | 建议：以方向摘要 + 引导占位开头为正文初稿，用户可提交前补写 |
| R-4 | 追加章末的锚点限制：允许任意章节追加？ | 建议：允许（用户显式选择），追加处变更审阅可见 |
| R-5 | 推演并发与 chat 任务是否共享运行注册表 | 建议：独立轻量注册表（continuation/task.go），不复用 chat 任务编排 |
| R-6 | 材料导入是否进 workspacechange 账本 | 建议：不进（非 Agent 编辑），但记录日志 + 备份 |
| R-7 | 配置持久化作用域 | 延续项目约定：写作/游戏通用偏好存用户配置（settings 体系） |

评审通过后按 §2/§3 落地编码。
