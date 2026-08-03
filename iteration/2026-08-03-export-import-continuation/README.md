# 迭代归档：2026-08（作品目录导入导出 · Skills 炼成助手 · 续写推演）

> 迭代目录：`iteration/` · 关联分支：`feature/client_pack` · 基线版本：v0.3.0

## 进度状态表

| # | 产物 | 状态 | 备注 |
|---|---|---|---|
| 1 | 《01-requirements.md》原始需求梳理（含现状分析） | ✅ 完成 | 5 项需求 + 代码调研结论 |
| 2 | 需求歧义确认 | ✅ 完成 | 导出范围自选/仅浏览器下载/导入双模式/推演粒度与三级深度 |
| 3 | 《02-prd.md》PRD | ✅ 冻结 | 含导出勾选、材料导入兜底、Skills 草稿流水线、推演 短/中/长 |
| 4 | 《03-prd-review.md》PRD 评审 | ✅ 完成 | 6 项决策点全部确认 |
| 5 | 《04-test-cases.md》测试用例 | ✅ 完成 | 57 条（U/I/M），含回归用例 |
| 6 | 《05-implementation-plan.md》实现方案 | ✅ 完成 | 4 个 Phase 全部落地 |
| 7 | 《06-technical-design.md》技术方案 | ✅ 完成 | 新子包 continuation + drafts + SSE 协议 + API 路由 |
| 8 | 《07-design-review.md》实现与技术方案评审 | ✅ 完成 | R-1~R-7 定案 + Q1/Q2 确认 |
| 9 | 《08-test-regression.md》测试回归验证 | 🔄 自动验证完成 | 后端 36 包 + 前端 645 用例全绿；手动浏览器验证待用户执行 |

## 需求覆盖矩阵

| 需求 | PRD 章节 | 实现 Phase | 测试用例 |
|---|---|---|---|
| 1 作品目录导出（勾选范围/格式） | 1.1-1.3 | Phase 1 | EXP-001~008 |
| 2 作品目录导入（格式+材料） | 2.1-2.3 | Phase 2 | IMP-001~008 |
| 3 书籍设定导入导出（大纲/规则/粗细纲） | 3.1-3.3 | Phase 2 | SET-001~007 |
| 4 Skills AI 对话页（生成→草稿→导入流水线） | 4.1-4.3 | Phase 3 | SKL-001~007 |
| 5 续写推演（短/中/长、方向/片段、选择/修改/提交） | 5.1-5.8 | Phase 4 | CON-001~018 |

## 关键决策记录（摘要）

- 导出内容由用户 checkbox 勾选（章节/细纲最新或全部/大纲/规则/进度/角色状态/灵感/元数据），zip 保留原目录结构；下载仅浏览器默认方式。
- 材料导入识别失败 → 用户手动选择目标类型；写入前自动备份到 `.casemagica/backups/settings-import/`，不进变更审阅账本。
- Skill 草稿区为用户级 `<denovaDir>/skill-drafts/`；agent 通过 `create_skill_draft` 工具产出，前端工具结果卡片驱动"查看/编辑/导入/丢弃"流水线。
- 续写推演：逐候选独立调用（小并发 2 路），SSE 渐进输出；结果落盘 `<ws>/.casemagica/continuations/<task-id>/`，刷新可恢复；方向级候选提交时由用户选择"摘要作初稿/空章节"。
- 新增配置项：`continuation.max_candidates / excerpt_max_chars / prefix_chars / timeout_minutes`（默认不限时）。
