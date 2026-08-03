package continuation

import (
	"fmt"
	"os"
	"strings"

	"casemagica/internal/book"
	"casemagica/internal/styleref"
	"casemagica/internal/workspacepath"
)

// 各上下文片段的硬上限（PRD 5.5，均有明确来源与大小约束）。
const (
	maxOutlineRunes       = 64 * 1024
	maxRulesRunes         = 64 * 1024
	maxProgressRunes      = 32 * 1024
	maxCharacterRunes     = 32 * 1024
	maxGroupRunes         = 32 * 1024
	maxPreferenceRunes    = 2000
	maxCustomStyleRunes   = 20000
)

// BoundedContext 是一次推演所需的全部有界上下文。
type BoundedContext struct {
	AnchorChapter string   // 锚点章节路径（空表示无锚点）
	AnchorTitle   string   // 锚点章节显示标题
	Outline       string   // 长期大纲
	Rules         string   // 创作规则 CREATOR.md
	Progress      string   // 写作进度
	CharacterState string  // 角色状态
	ChapterGroups string   // 章节组细纲
	Preceding     []string // 前文正文窗口（锚点章 + 前 1~2 章，每段有上限）
	Styles        string   // 风格参考内容
	CustomStyle   string   // 用户自定义文风
	Preference    string   // 方向偏好
	AnchorAnchor  bool
}

// BuildBoundedContext 组装有界推演上下文。每个片段都有硬上限，避免无限增长内容进入提示词。
// prefixChars 为单章前文注入上限（字），styleRefNames 来自 .casemagica/styles 文风参考。
func BuildBoundedContext(service *book.Service, request ExploreRequest, prefixChars int) (BoundedContext, error) {
	ctx := BoundedContext{
		AnchorChapter: strings.TrimSpace(request.AnchorChapter),
		CustomStyle:   truncateRunes(request.CustomStyle, maxCustomStyleRunes),
		Preference:    truncateRunes(request.Preference, maxPreferenceRunes),
	}
	summary, err := service.Summary()
	if err != nil {
		return ctx, err
	}
	ctx.Outline = readBounded(service, "setting/outline.md", maxOutlineRunes)
	ctx.Rules = readBounded(service, "CREATOR.md", maxRulesRunes)
	ctx.Progress = readBounded(service, "setting/progress.md", maxProgressRunes)
	ctx.CharacterState = readBounded(service, "setting/character-states.md", maxCharacterRunes)

	// 细纲：优先锚点章节所在组，否则取最新一组
	ctx.ChapterGroups = boundedChapterGroups(service, summary.ChapterPlans, ctx.AnchorChapter)

	// 前文窗口：锚点章节 + 前 1~2 章
	anchorIndex := -1
	for i, chapter := range summary.Chapters {
		if chapter.Path == ctx.AnchorChapter {
			anchorIndex = i
			break
		}
	}
	if anchorIndex < 0 && len(summary.Chapters) > 0 {
		anchorIndex = len(summary.Chapters) - 1
		ctx.AnchorChapter = summary.Chapters[anchorIndex].Path
	}
	if anchorIndex >= 0 {
		ctx.AnchorTitle = summary.Chapters[anchorIndex].DisplayTitle
	}
	start := anchorIndex - 2
	if start < 0 {
		start = 0
	}
	for i := start; i <= anchorIndex && i >= 0 && i < len(summary.Chapters); i++ {
		chapter := summary.Chapters[i]
		content, err := service.ReadFile(chapter.Path)
		if err != nil {
			continue
		}
		window := truncateRunes(content, prefixChars)
		if strings.TrimSpace(window) == "" {
			continue
		}
		ctx.Preceding = append(ctx.Preceding, fmt.Sprintf("【%s】\n%s", chapter.DisplayTitle, window))
	}

	if len(request.StyleRefNames) > 0 {
		ctx.Styles = readStyleReferences(service, request.StyleRefNames)
	}
	return ctx, nil
}

func readBounded(service *book.Service, rel string, limit int) string {
	content, err := service.ReadFile(rel)
	if err != nil {
		return ""
	}
	return truncateRunes(content, limit)
}

// boundedChapterGroups 组装细纲（有界）：取最新一组细纲内容。
func boundedChapterGroups(service *book.Service, plans []book.DocumentPreview, anchorChapter string) string {
	if len(plans) == 0 {
		return ""
	}
	latest := plans[len(plans)-1]
	return readBounded(service, latest.Path, maxGroupRunes)
}

func readStyleReferences(service *book.Service, names []string) string {
	workspace := service.Workspace()
	library := styleref.NewLibrary(workspacepath.Dir(workspace))
	refs, err := library.List()
	if err != nil {
		return ""
	}
	byName := map[string]styleref.Reference{}
	for _, ref := range refs {
		byName[ref.Name] = ref
	}
	var sb strings.Builder
	seen := map[string]bool{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		ref, ok := byName[name]
		if !ok || ref.Missing {
			continue
		}
		data, err := os.ReadFile(ref.Path)
		if err != nil {
			continue
		}
		sb.WriteString(fmt.Sprintf("## 文风参考：%s\n%s\n\n", name, truncateRunes(string(data), 160*1024)))
	}
	return strings.TrimSpace(sb.String())
}

func truncateRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit])
}
