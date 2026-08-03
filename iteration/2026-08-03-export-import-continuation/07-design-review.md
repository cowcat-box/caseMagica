# 实现与技术方案评审记录

> 迭代：2026-08 · 状态：待确认 · 关联：《05-implementation-plan.md》《06-technical-design.md》

---

## 一、技术自检结论（已通过）

1. **SSE 模式确认**：推演接口复用 `handler_novel_import.go` 的 `io.Pipe + writeEvent` 模式（handler_novel_import.go:52-113），不依赖 chat 任务注册表；事件格式对齐 `agent.Event{Type, Data}`。
2. **独立模型调用模式确认**：`automation_trigger.go:28`（`chatModelConfigForAgent` + `openai.NewChatModel` + `cm.Generate` + `withStandaloneRunTrace` + `beginLLMCallTrace` + json_mode + plain retry）正是推演所需的"有界单次生成"模式；流式可用 `cm.Stream`（chat_stream.go 的 recvMessageFrame 模式）。
3. **workspacechange 写入确认**：`ApplyEdits`（含 BaseRevision 校验）与 `ReplaceFile`（支持新建）可直接用于提交落盘；变更自动进入 Change Review 账本。
4. **Skill 用户级目录确认**：user scope = `<denovaDir>/skills`（directories.go:17-21），草稿区 `<denovaDir>/skill-drafts` 同级放置，跨工作区共享。
5. **配置扩展确认**：`config/config.go` Config 结构体 + `config.toml` 可平铺新增 `continuation` 段。
6. **导出 zip 依赖**：标准库 `archive/zip`，无需新增依赖（skills install 已有先例）。

## 二、关键决策定案

| # | 问题 | 定案 |
|---|---|---|
| R-1 | 推演 LLM 调用入口 | 在 `internal/agent` 新增导出助手 `GenerateStandaloneJSON(ctx, cfg, agentKind, source, mode, systemPrompt, instruction)`：封装 trace 初始化、json_mode、plain-text 重试（对齐 tool_agent.go 模式）。推演包仅依赖此出口；现有 4 处同模式实现本轮不重构（避免回归）。该助手被推演按候选×次数调用，非"只调用一次的小 helper"，符合项目规范 |
| R-2 | `create_skill_draft` 工具通知前端 | 复用工具结果链路（方案 A）：工具返回结构化 JSON，前端从 tool_result 渲染"Skill 草稿就绪"卡片；不新增事件类型 |
| R-3 | 方向级候选提交为新章节的正文 | **待用户确认**（见下） |
| R-4 | 追加章末锚点 | 允许任意章节追加（用户显式选择），变更审阅可见 |
| R-5 | 推演任务注册表 | 独立轻量注册表（`continuation/task.go`），不复用 chat 任务编排；同工作区单任务互斥 |
| R-6 | 材料导入是否进账本 | **待用户确认**（见下） |
| R-7 | 推演配置持久化 | 前端 localStorage（对齐 TabController 桶式持久化先例），不进服务端用户配置 |

## 三、评审中发现并已吸收的问题

1. **导出 API 兼容性**：`GET /api/books/export` 保持无 selection 参数时行为不变（HomeView 不受影响）。
2. **设定导出安全**：`GET /api/books/settings/export?path=` 必须白名单校验路径（仅允许 6 类设定路径 + chapter-groups 前缀），防任意文件读取。
3. **材料导入限制**：扩展名 `.md`；大小上限对齐既有上传限制（64MB，novel_import 先例）。
4. **方向级候选无正文**：提交"新建章节"时若用户未补写正文，以标题+方向摘要为初稿内容（见待确认 R-3）。
5. **锚点校验**：`anchor_chapter_path` 必须来自 `book.Summary()` 章节枚举，拒绝任意路径（防越权读取）。
6. **推演流式**：单个候选使用 `cm.Stream` 渐进输出 `candidate_delta`，前端卡片实时增长；失败候选立即置 failed 不影响其余。
7. **并发安全**：候选 goroutine 全部 `defer recover`；任务级 `context.WithCancel` 支持中止。
8. **细纲"最新/全部"来源**：复用 `summary.chapter_plans`（与 ChapterOutline 同一数据源），无需新枚举逻辑。

## 四、遗留问题（请确认）

| # | 问题 | 选项 |
|---|---|---|
| Q1 (R-3) | 方向级候选提交为"新建章节"时，正文初稿怎么写？ | ✅ **提供选项让用户决定**：提交弹窗提供"以方向摘要为正文初稿 / 创建空章节（仅标题）"二选一 |
| Q2 (R-6) | 材料导入（覆盖书籍设定文件）是否进入变更审阅（Change Review）账本？ | ✅ **不进账本**：仅备份 + 日志，避免审阅噪音 |

以上决策已合入《02-prd.md》相应小节（5.4 提交流程补充"初稿选项"、2.3 导入确认注明备份路径）。方案冻结。
