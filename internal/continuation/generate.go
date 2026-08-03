package continuation

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"casemagica/config"
	"casemagica/internal/agent"
	"casemagica/internal/book"
)

// ExploreHandler 是推演过程中的事件回调（由 API 层驱动 SSE）。
type ExploreHandler interface {
	OnCandidateStart(task *Task, index int)
	OnCandidateDelta(task *Task, index int, content string)
	OnCandidateDone(task *Task, candidate Candidate)
	OnCandidateFailed(task *Task, index int, message string)
	OnExploreDone(task *Task)
}

// Generator 执行一次推演的候选生成。
type Generator struct {
	cfg         *config.Config
	service     *book.Service
	concurrency int
	generate    func(ctx context.Context, cfg *config.Config, spec agent.GenerateStandaloneSpec) (string, error)
}

// NewGenerator 创建候选生成器；concurrency <=0 时使用 2。
func NewGenerator(cfg *config.Config, service *book.Service, concurrency int) *Generator {
	if concurrency <= 0 {
		concurrency = 2
	}
	return &Generator{cfg: cfg, service: service, concurrency: concurrency}
}

// SetGenerateFuncForTest 替换候选生成函数（仅测试使用）。
func (g *Generator) SetGenerateFuncForTest(fn func(ctx context.Context, cfg *config.Config, spec agent.GenerateStandaloneSpec) (string, error)) {
	g.generate = fn
}

// Generate 生成全部候选（逐候选独立调用，小并发，部分失败不影响其他）。
// 返回任务最终状态。ctx 取消（中止）时剩余候选标记 cancelled 并返回。
func (g *Generator) Generate(ctx context.Context, task *Task, bounded BoundedContext, mode Mode, excerptMax int, handler ExploreHandler) TaskStatus {
	task.SetStatus(TaskStatusRunning)
	candidateCount := DefaultCandidateCount(Depth(task.Request.Depth), g.maxCandidates())

	sem := make(chan struct{}, g.concurrency)
	var wg sync.WaitGroup
	for index := 0; index < candidateCount; index++ {
		index := index
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if recovered := recover(); recovered != nil {
					log.Printf("[continuation] candidate panic recovered task=%s index=%d err=%v", task.ID, index, recovered)
					failed := Candidate{Index: index, Status: string(CandidateFailed), Error: fmt.Sprint(recovered)}
					task.UpdateCandidate(failed)
					handler.OnCandidateFailed(task, index, fmt.Sprint(recovered))
				}
			}()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				failed := Candidate{Index: index, Status: string(CandidateFailed), Error: "已中止"}
				task.UpdateCandidate(failed)
				handler.OnCandidateFailed(task, index, "已中止")
				return
			}
			if ctx.Err() != nil {
				failed := Candidate{Index: index, Status: string(CandidateFailed), Error: "已中止"}
				task.UpdateCandidate(failed)
				handler.OnCandidateFailed(task, index, "已中止")
				return
			}
			handler.OnCandidateStart(task, index)
			task.UpdateCandidate(Candidate{Index: index, Status: string(CandidateGenerating)})
			g.generateOne(ctx, task, index, bounded, mode, excerptMax, handler)
		}()
	}
	wg.Wait()
	if ctx.Err() != nil {
		task.SetStatus(TaskStatusCancelled)
		return TaskStatusCancelled
	}
	task.Complete()
	status, _ := task.StatusNow()
	return status
}

func (g *Generator) maxCandidates() int {
	if g.cfg == nil || g.cfg.Continuation.MaxCandidates == nil {
		return DefaultContinuationMaxCandidates()
	}
	return *g.cfg.Continuation.MaxCandidates
}

// DefaultContinuationMaxCandidates 是默认最大候选数（避免与 config 包循环依赖）。
func DefaultContinuationMaxCandidates() int {
	return 3
}

func (g *Generator) generateOne(ctx context.Context, task *Task, index int, bounded BoundedContext, mode Mode, excerptMax int, handler ExploreHandler) {
	spec := agent.GenerateStandaloneSpec{
		AgentKind:   "ide",
		Source:      "continuation",
		Mode:        fmt.Sprintf("candidate_%d_%s", index, mode),
		System:      continuationSystemPrompt(mode, Depth(task.Request.Depth), excerptMax),
		Instruction: buildInstruction(task.Request, bounded, mode),
		JSONMode:    true,
	}
	output, err := g.callModel(ctx, spec)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		log.Printf("[continuation] candidate generate failed task=%s index=%d err=%v", task.ID, index, err)
		failed := Candidate{Index: index, Status: string(CandidateFailed), Error: err.Error()}
		task.UpdateCandidate(failed)
		handler.OnCandidateFailed(task, index, err.Error())
		return
	}
	candidate, parseErr := parseCandidate(output, index, mode)
	if parseErr != nil {
		if ctx.Err() != nil {
			return
		}
		log.Printf("[continuation] candidate parse failed task=%s index=%d err=%v", task.ID, index, parseErr)
		failed := Candidate{Index: index, Status: string(CandidateFailed), Error: parseErr.Error()}
		task.UpdateCandidate(failed)
		handler.OnCandidateFailed(task, index, parseErr.Error())
		return
	}
	task.UpdateCandidate(candidate)
	handler.OnCandidateDone(task, candidate)
}

func (g *Generator) callModel(ctx context.Context, spec agent.GenerateStandaloneSpec) (string, error) {
	if g.generate != nil {
		return g.generate(ctx, g.cfg, spec)
	}
	return agent.GenerateStandalone(ctx, g.cfg, spec)
}

func continuationSystemPrompt(mode Mode, depth Depth, excerptMax int) string {
	var b strings.Builder
	b.WriteString("你是 CaseMagica 的续写推演器。你的唯一任务是基于给定的有界创作上下文，为作品的下一章推演一个独立的续写候选。")
	b.WriteString("\n规则：")
	b.WriteString("\n1. 严格保持已有角色、世界观、设定与文风的一致性；不要重复前文中已经发生的情节。")
	b.WriteString("\n2. 候选必须是对下一章的直接续写，而不是大纲摘要。")
	b.WriteString("\n3. 只输出一个 JSON object，不要输出任何其他内容（不要代码块包裹）。")
	switch depth {
	case DepthLong:
		b.WriteString("\n4. JSON schema: {\"title\":\"章节标题建议\",\"direction\":\"剧情方向摘要（3~5句）\",\"excerpt\":\"正文开头片段（完整成段）\",\"plan\":\"后续3~5章走向规划（要点式）\"}")
		if excerptMax > 0 {
			fmt.Fprintf(&b, "\n5. excerpt 长度控制在 %d 字以内。", excerptMax)
		}
	case DepthShort:
		b.WriteString("\n4. JSON schema: {\"title\":\"章节标题建议\",\"direction\":\"剧情方向摘要（3~5句）\",\"rationale\":\"推演理由（1~2句）\"}")
	default:
		if mode == ModeDirection {
			b.WriteString("\n4. JSON schema: {\"title\":\"章节标题建议\",\"direction\":\"剧情方向摘要（3~5句）\",\"rationale\":\"推演理由（1~2句）\"}")
		} else {
			b.WriteString("\n4. JSON schema: {\"title\":\"章节标题建议\",\"direction\":\"剧情方向摘要（3~5句）\",\"excerpt\":\"正文开头片段（完整成段）\"}")
			if excerptMax > 0 {
				fmt.Fprintf(&b, "\n5. excerpt 长度控制在 %d 字以内。", excerptMax)
			}
		}
	}
	b.WriteString("\n5. title 与正文使用作品当前语言。")
	return b.String()
}

// parseCandidate 解析单个候选的 LLM JSON 输出；失败返回错误（不阻塞其他候选）。
func parseCandidate(output string, index int, mode Mode) (Candidate, error) {
	content := extractJSONContent(output)
	var payload struct {
		Title     string `json:"title"`
		Direction string `json:"direction"`
		Excerpt   string `json:"excerpt"`
		Plan      string `json:"plan"`
		Rationale string `json:"rationale"`
	}
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		return Candidate{}, fmt.Errorf("候选输出不是有效 JSON: %v", err)
	}
	candidate := Candidate{
		Index:     index,
		Title:     strings.TrimSpace(payload.Title),
		Direction: strings.TrimSpace(payload.Direction),
		Excerpt:   strings.TrimSpace(payload.Excerpt),
		Plan:      strings.TrimSpace(payload.Plan),
		Status:    string(CandidateDone),
	}
	if candidate.Title == "" && candidate.Direction == "" && candidate.Excerpt == "" {
		return Candidate{}, fmt.Errorf("候选输出为空")
	}
	return candidate, nil
}

// extractJSONContent 提取模型输出中的 JSON（容错代码块包裹与首尾噪声）。
func extractJSONContent(content string) string {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSpace(content)
		content = strings.TrimSuffix(content, "```")
		return strings.TrimSpace(content)
	}
	start := strings.Index(content, "{")
	if start > 0 {
		end := strings.LastIndex(content, "}")
		if end > start {
			return strings.TrimSpace(content[start : end+1])
		}
	}
	return content
}

func buildInstruction(request ExploreRequest, bounded BoundedContext, mode Mode) string {
	var b strings.Builder
	b.WriteString("请基于以下上下文推演续写候选。\n\n")

	writeSection := func(title, content string) {
		content = strings.TrimSpace(content)
		if content == "" {
			return
		}
		fmt.Fprintf(&b, "## %s\n%s\n\n", title, content)
	}
	writeSection("长期大纲", bounded.Outline)
	writeSection("创作规则", bounded.Rules)
	writeSection("写作进度", bounded.Progress)
	writeSection("角色状态", bounded.CharacterState)
	writeSection("章节组细纲", bounded.ChapterGroups)
	writeSection("文风参考", bounded.Styles)
	if bounded.CustomStyle != "" {
		writeSection("自定义文风要求", bounded.CustomStyle)
	}
	if len(bounded.Preceding) > 0 {
		writeSection("前文正文（截至锚点章节）", strings.Join(bounded.Preceding, "\n\n"))
	}
	if bounded.Preference != "" {
		writeSection("推演方向偏好", bounded.Preference)
	}
	if mode == ModeExcerpt {
		b.WriteString("请输出片段级候选：包含可直接作为下一章开头的正文片段。\n")
	} else {
		b.WriteString("请输出方向级候选：包含章节标题建议与剧情方向摘要。\n")
	}
	return b.String()
}
