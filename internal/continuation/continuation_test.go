package continuation

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"casemagica/config"
	"casemagica/internal/agent"
	"casemagica/internal/book"
	"casemagica/internal/workspacechange"
)

// stubGenerate 构造一个固定返回的候选生成函数（JSON 模式）。
func stubGenerate(payload string) func(context.Context, *config.Config, agent.GenerateStandaloneSpec) (string, error) {
	return func(_ context.Context, _ *config.Config, _ agent.GenerateStandaloneSpec) (string, error) {
		return payload, nil
	}
}

func TestNormalizeDepthAndMode(t *testing.T) {
	cases := []struct {
		depth, mode string
		okDepth     bool
		okMode      bool
		wantMode    Mode
	}{
		{"short", "direction", true, true, ModeDirection},
		{"short", "excerpt", true, false, ModeExcerpt},
		{"medium", "excerpt", true, true, ModeExcerpt},
		{"long", "direction", true, true, ModeDirection},
		{"bogus", "excerpt", false, false, ModeExcerpt},
	}
	for _, tc := range cases {
		depth, ok := NormalizeDepth(tc.depth)
		if ok != tc.okDepth {
			t.Errorf("NormalizeDepth(%q) ok = %v", tc.depth, ok)
			continue
		}
		if !ok {
			continue
		}
		mode, ok := NormalizeMode(tc.mode, depth)
		if ok != tc.okMode || (ok && mode != tc.wantMode) {
			t.Errorf("NormalizeMode(%q, %q) = (%v, %v), want (%v, %v)", tc.mode, tc.depth, mode, ok, tc.wantMode, tc.okMode)
		}
	}
}

func TestDefaultCandidateCount(t *testing.T) {
	if got := DefaultCandidateCount(DepthShort, 3); got != 2 {
		t.Fatalf("short count = %d", got)
	}
	if got := DefaultCandidateCount(DepthMedium, 3); got != 3 {
		t.Fatalf("medium count = %d", got)
	}
	if got := DefaultCandidateCount(DepthMedium, 1); got != 1 {
		t.Fatalf("capped count = %d", got)
	}
}

func TestParseCandidate(t *testing.T) {
	candidate, err := parseCandidate(`{"title":"第4章 转折","direction":"主角发现真相","excerpt":"夜色中……","plan":"后续走向"}`, 2, ModeExcerpt)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Index != 2 || candidate.Title != "第4章 转折" || candidate.Status != string(CandidateDone) {
		t.Fatalf("candidate mismatch: %#v", candidate)
	}
	// 容错：代码块包裹 + 前后噪声
	candidate, err = parseCandidate("```json\n{\"title\":\"t\",\"direction\":\"d\"}\n```", 1, ModeDirection)
	if err != nil || candidate.Title != "t" {
		t.Fatalf("fenced parse failed: %#v err=%v", candidate, err)
	}
	if _, err := parseCandidate("not json at all", 0, ModeDirection); err == nil {
		t.Fatalf("invalid json should fail")
	}
}

func TestBuildBoundedContextLimits(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "setting", "chapter-groups"), 0o755); err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("很长很长很长很长很长很长很长很长很长", 2000)
	files := map[string]string{
		"chapters/ch00001-第一章-开局.md":  "第一章 开局\n\n" + long,
		"chapters/ch00002-第二章-追光.md":  "第二章 追光\n\n" + long,
		"chapters/ch00003-第三章-汇聚.md":  "第三章 汇聚\n\n" + long,
		"setting/outline.md":              strings.Repeat("大纲内容", 20000),
		"setting/chapter-groups/group1.md": "细纲内容",
	}
	for path, content := range files {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(path)), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	service := book.NewService(root)
	ctx, err := BuildBoundedContext(service, ExploreRequest{
		Workspace:     root,
		AnchorChapter: "chapters/ch00003-第三章-汇聚.md",
	}, 3000)
	if err != nil {
		t.Fatal(err)
	}
	if ctx.AnchorChapter != "chapters/ch00003-第三章-汇聚.md" {
		t.Fatalf("anchor = %q", ctx.AnchorChapter)
	}
	if len(ctx.Preceding) == 0 {
		t.Fatalf("preceding should include anchor chapter window")
	}
	for _, window := range ctx.Preceding {
		if len([]rune(window)) > 3000+100 {
			t.Fatalf("window exceeds prefix limit: %d", len([]rune(window)))
		}
	}
	if len([]rune(ctx.Outline)) > maxOutlineRunes {
		t.Fatalf("outline exceeds limit")
	}
	if ctx.ChapterGroups == "" {
		t.Fatalf("chapter groups should be included")
	}
}

func TestTaskRegistryEnforcesSingleActiveTask(t *testing.T) {
	root := t.TempDir()
	service := NewService(root)
	first, err := service.Create(ExploreRequest{Workspace: root, Depth: "medium", Mode: "direction"}, func() {})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(ExploreRequest{Workspace: root, Depth: "medium", Mode: "direction"}, func() {}); err != ErrTaskInProgress {
		t.Fatalf("second task err = %v, want ErrTaskInProgress", err)
	}
	first.Cancel()
	second, err := service.Create(ExploreRequest{Workspace: root, Depth: "medium", Mode: "direction"}, func() {})
	if err != nil {
		t.Fatalf("create after cancel failed: %v", err)
	}
	_ = second
}

func TestTaskPersistenceAndRestore(t *testing.T) {
	root := t.TempDir()
	service := NewService(root)
	task, err := service.Create(ExploreRequest{Workspace: root, Depth: "medium", Mode: "excerpt"}, func() {})
	if err != nil {
		t.Fatal(err)
	}
	task.UpdateCandidate(Candidate{Index: 0, Title: "t", Direction: "d", Excerpt: "e", Status: string(CandidateDone)})
	task.SetStatus(TaskStatusDone)

	restored, err := service.Get(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Status != TaskStatusDone {
		t.Fatalf("restored status = %s", restored.Status)
	}
	candidate, ok := restored.Candidate(0)
	if !ok || candidate.Excerpt != "e" {
		t.Fatalf("restored candidate = %#v", candidate)
	}

	metas, err := service.Store().ListMetas()
	if err != nil || len(metas) != 1 {
		t.Fatalf("list metas = %v err=%v", metas, err)
	}
}

func TestTaskDeleteMovesToBackup(t *testing.T) {
	root := t.TempDir()
	service := NewService(root)
	task, err := service.Create(ExploreRequest{Workspace: root, Depth: "medium", Mode: "direction"}, func() {})
	if err != nil {
		t.Fatal(err)
	}
	task.SetStatus(TaskStatusDone)
	backupPath, err := service.Delete(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(backupPath, "backups") {
		t.Fatalf("backup = %q", backupPath)
	}
	if _, err := os.Stat(filepath.Join(root, ".casemagica", "continuations", task.ID)); !os.IsNotExist(err) {
		t.Fatalf("task dir should be removed")
	}
}

func TestNextChapterFilename(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "chapters", "ch00003-第三章-汇聚.md"), []byte("内容"), 0o644); err != nil {
		t.Fatal(err)
	}
	rel, err := book.NewService(root).NextChapterFilename("第4章 新起点/高潮")
	if err != nil {
		t.Fatal(err)
	}
	if rel != "chapters/ch00004-第4章-新起点-高潮.md" {
		t.Fatalf("next filename = %q", rel)
	}
}

func TestCommitterAppendAndNewChapter(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	anchor := "chapters/ch00001-第一章.md"
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(anchor)), []byte("第一章 开局\n\n天亮了。"), 0o644); err != nil {
		t.Fatal(err)
	}
	changeService, err := workspacechange.NewService(root)
	if err != nil {
		t.Fatal(err)
	}
	committer := NewCommitter(book.NewService(root), changeService)
	task := &Task{
		ID:        "t1",
		Workspace: root,
		Request:   ExploreRequest{Workspace: root, AnchorChapter: anchor},
		Status:    TaskStatusDone,
		candidates: map[int]*Candidate{
			0: {Index: 0, Title: "第2章 追光", Direction: "推进", Excerpt: "林川踏入雨夜。", Status: string(CandidateDone)},
		},
	}

	// 追加章末
	result, err := committer.Commit(CommitRequest{TaskID: "t1", Index: 0, Title: "第2章 追光", Content: "林川踏入雨夜。", Target: string(TargetAppend)}, task)
	if err != nil {
		t.Fatal(err)
	}
	if result.Path != anchor {
		t.Fatalf("append path = %q", result.Path)
	}
	content, err := book.NewService(root).ReadFile(anchor)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "林川踏入雨夜。") || !strings.Contains(content, "天亮了。") {
		t.Fatalf("append content mismatch:\n%s", content)
	}

	// 新建章节
	result, err = committer.Commit(CommitRequest{TaskID: "t1", Index: 0, Title: "第2章 追光", Content: "第二章 追光\n\n林川踏入雨夜。", Target: string(TargetNewChapter)}, task)
	if err != nil {
		t.Fatal(err)
	}
	if result.Path != "chapters/ch00002-第2章-追光.md" {
		t.Fatalf("new chapter path = %q", result.Path)
	}
	newContent, err := book.NewService(root).ReadFile(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(newContent, "林川踏入雨夜。") {
		t.Fatalf("new chapter content mismatch:\n%s", newContent)
	}
}

func TestGeneratorPartialFailureTolerance(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "chapters", "ch00001-第一章.md"), []byte("第一章 开局"), 0o644); err != nil {
		t.Fatal(err)
	}
	service := NewService(root)
	var mu sync.Mutex
	callCount := 0
	task, err := service.Create(ExploreRequest{Workspace: root, Depth: "medium", Mode: "excerpt"}, func() {})
	if err != nil {
		t.Fatal(err)
	}
	generator := NewGenerator(nil, book.NewService(root), 2)
	generator.SetGenerateFuncForTest(func(_ context.Context, _ *config.Config, _ agent.GenerateStandaloneSpec) (string, error) {
		mu.Lock()
		callCount++
		current := callCount
		mu.Unlock()
		if current%2 == 0 {
			return "", os.ErrNotExist
		}
		payload, _ := json.Marshal(map[string]string{"title": "t", "direction": "d", "excerpt": "e"})
		return string(payload), nil
	})
	bounded := BoundedContext{AnchorChapter: "chapters/ch00001-第一章.md"}
	handler := &recordingHandler{taskID: task.ID}
	status := generator.Generate(context.Background(), task, bounded, ModeExcerpt, 1500, handler)
	if status != TaskStatusPartiallyDone {
		t.Fatalf("status = %s, want partially_done", status)
	}
	mu.Lock()
	count := callCount
	mu.Unlock()
	if count != 3 {
		t.Fatalf("call count = %d, want 3", count)
	}
	if handler.doneCount != 2 || handler.failedCount != 1 {
		t.Fatalf("handler done=%d failed=%d", handler.doneCount, handler.failedCount)
	}
}

type recordingHandler struct {
	taskID      string
	doneCount   int
	failedCount int
}

func (h *recordingHandler) OnCandidateStart(task *Task, index int) {}
func (h *recordingHandler) OnCandidateDelta(task *Task, index int, content string) {
}
func (h *recordingHandler) OnCandidateDone(task *Task, candidate Candidate) {
	h.doneCount++
}
func (h *recordingHandler) OnCandidateFailed(task *Task, index int, message string) {
	h.failedCount++
}
func (h *recordingHandler) OnExploreDone(task *Task) {}
