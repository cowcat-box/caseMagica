package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"casemagica/config"
	"casemagica/internal/agent"
	"casemagica/internal/book"
	"casemagica/internal/continuation"
)

// continuationStubFactory 返回一个固定输出候选的生成器工厂。
func continuationStubFactory(payloads []string) func(*config.Config, *book.Service, int) *continuation.Generator {
	return func(_ *config.Config, service *book.Service, concurrency int) *continuation.Generator {
		generator := continuation.NewGenerator(nil, service, concurrency)
		var mu = struct{ index int }{}
		generator.SetGenerateFuncForTest(func(_ context.Context, _ *config.Config, _ agent.GenerateStandaloneSpec) (string, error) {
			mu.index++
			if mu.index-1 < len(payloads) {
				return payloads[mu.index-1], nil
			}
			return `{"title":"额外","direction":"额外方向"}`, nil
		})
		return generator
	}
}

func readSSEEvents(t *testing.T, body string) []map[string]any {
	t.Helper()
	events := []map[string]any{}
	scanner := bufio.NewScanner(strings.NewReader(body))
	var current map[string]any
	var dataType string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			dataType = strings.TrimPrefix(line, "event: ")
			current = map[string]any{"event": dataType}
			continue
		}
		if strings.HasPrefix(line, "data: ") {
			var payload map[string]any
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &payload); err == nil && current != nil {
				for key, value := range payload {
					current[key] = value
				}
			}
			events = append(events, current)
			current = nil
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		// 单行 JSON（Hertz 测试场景可能压缩事件）
		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err == nil {
			events = append(events, payload)
		}
	}
	return events
}

func TestContinuationExploreAndStreamAPI(t *testing.T) {
	application := newTestApplication(t)
	if err := application.BookService().Create("chapters/ch00001-第一章-开局.md", "file", "第一章 开局\n\n天亮了。"); err != nil {
		t.Fatal(err)
	}
	application.SetContinuationGeneratorFactoryForTest(continuationStubFactory([]string{
		`{"title":"第2章 追光","direction":"林川进入雨夜","excerpt":"林川踏入雨夜。","plan":"后续走向"}`,
		`{"title":"第2章 暗潮","direction":"幕后黑手浮现","excerpt":"阁楼的灯亮了起来。"}`,
	}))
	server := NewServer(application, "0")
	workspace := application.Workspace()

	resp := performJSONRequest(t, server, http.MethodPost, "/api/continuation/explore", map[string]any{
		"workspace":          workspace,
		"anchor_chapter_path": "chapters/ch00001-第一章-开局.md",
		"depth":              "medium",
		"mode":               "excerpt",
		"preference":         "向悬疑推进",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("explore status = %d body=%s", resp.Code, resp.Body.String())
	}
	events := readSSEEvents(t, resp.Body.String())
	var taskID string
	doneCount := 0
	var finalStatus string
	for _, event := range events {
		switch event["event"] {
		case "explore_start":
			taskID, _ = event["task_id"].(string)
		case "candidate_done":
			doneCount++
		case "explore_done":
			finalStatus, _ = event["status"].(string)
		}
	}
	if taskID == "" {
		t.Fatalf("no task_id in events: %v", events)
	}
	if doneCount != 3 {
		t.Fatalf("candidate_done = %d, events=%v", doneCount, events)
	}
	if finalStatus != "done" {
		t.Fatalf("final status = %q, events=%v", finalStatus, events)
	}

	// 任务列表
	listResp := performJSONRequest(t, server, http.MethodGet, "/api/continuation/tasks?workspace="+url.QueryEscape(workspace), nil)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", listResp.Code, listResp.Body.String())
	}
	var listBody struct {
		Tasks []continuation.TaskMeta `json:"tasks"`
	}
	decodeResponse(t, listResp.Body.Bytes(), &listBody)
	if len(listBody.Tasks) != 1 || listBody.Tasks[0].ID != taskID {
		t.Fatalf("task list mismatch: %#v", listBody.Tasks)
	}

	// 恢复流（快照）
	streamResp := performJSONRequest(t, server, http.MethodGet, "/api/continuation/tasks/"+taskID+"/stream", nil)
	if streamResp.Code != http.StatusOK {
		t.Fatalf("stream status = %d body=%s", streamResp.Code, streamResp.Body.String())
	}
	replayEvents := readSSEEvents(t, streamResp.Body.String())
	replayDone := 0
	for _, event := range replayEvents {
		if event["event"] == "candidate_done" {
			replayDone++
		}
	}
	if replayDone != 3 {
		t.Fatalf("replay candidate_done = %d", replayDone)
	}

	// 提交（新建章节）
	commitResp := performJSONRequest(t, server, http.MethodPost, "/api/continuation/commit", map[string]any{
		"task_id": taskID,
		"index":   0,
		"title":   "第2章 追光",
		"content": "第二章 追光\n\n林川踏入雨夜。",
		"target":  "new_chapter",
	})
	if commitResp.Code != http.StatusOK {
		t.Fatalf("commit status = %d body=%s", commitResp.Code, commitResp.Body.String())
	}
	var commitBody continuation.CommitResult
	decodeResponse(t, commitResp.Body.Bytes(), &commitBody)
	if commitBody.Path != "chapters/ch00002-第2章-追光.md" {
		t.Fatalf("commit path = %q", commitBody.Path)
	}
	content, err := application.BookService().ReadFile(commitBody.Path)
	if err != nil || !strings.Contains(content, "林川踏入雨夜。") {
		t.Fatalf("committed chapter mismatch: %q err=%v", content, err)
	}

	// 删除任务（备份）
	deleteResp := performJSONRequest(t, server, http.MethodDelete, "/api/continuation/tasks/"+taskID, nil)
	if deleteResp.Code != http.StatusOK {
		t.Fatalf("delete status = %d body=%s", deleteResp.Code, deleteResp.Body.String())
	}
	var deleteBody struct {
		BackupPath string `json:"backup_path"`
	}
	decodeResponse(t, deleteResp.Body.Bytes(), &deleteBody)
	if !strings.Contains(deleteBody.BackupPath, "backups") {
		t.Fatalf("backup path = %q", deleteBody.BackupPath)
	}
}

func TestContinuationExploreRejectsConcurrentTask(t *testing.T) {
	application := newTestApplication(t)
	application.SetContinuationGeneratorFactoryForTest(continuationStubFactory(nil))
	workspace := application.Workspace()

	first, err := application.ContinuationExplore(continuation.ExploreRequest{
		Workspace: workspace,
		Depth:     "short",
		Mode:      "direction",
	})
	if err != nil {
		t.Fatal(err)
	}
	// 进行中任务互斥（CON-007）
	if _, err := application.ContinuationExplore(continuation.ExploreRequest{
		Workspace: workspace,
		Depth:     "short",
		Mode:      "direction",
	}); err != continuation.ErrTaskInProgress {
		t.Fatalf("second explore err = %v, want ErrTaskInProgress", err)
	}
	first.Cancel()
	// 完成后允许新任务
	second, err := application.ContinuationExplore(continuation.ExploreRequest{
		Workspace: workspace,
		Depth:     "short",
		Mode:      "direction",
	})
	if err != nil {
		t.Fatalf("explore after cancel failed: %v", err)
	}
	_ = second
}

func TestContinuationExploreValidation(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	workspace := application.Workspace()

	// 非法深度
	resp := performJSONRequest(t, server, http.MethodPost, "/api/continuation/explore", map[string]any{
		"workspace": workspace,
		"depth":     "bogus",
		"mode":      "direction",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("invalid depth status = %d body=%s", resp.Code, resp.Body.String())
	}
	// 短推演 + 片段级
	resp = performJSONRequest(t, server, http.MethodPost, "/api/continuation/explore", map[string]any{
		"workspace": workspace,
		"depth":     "short",
		"mode":      "excerpt",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("short+excerpt status = %d body=%s", resp.Code, resp.Body.String())
	}
	// 不存在的锚点章节
	resp = performJSONRequest(t, server, http.MethodPost, "/api/continuation/explore", map[string]any{
		"workspace":           workspace,
		"anchor_chapter_path": "chapters/不存在.md",
		"depth":               "medium",
		"mode":                "direction",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("bad anchor status = %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestContinuationCommitRejectsIncompleteTask(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	workspace := application.Workspace()

	// 无任务
	resp := performJSONRequest(t, server, http.MethodPost, "/api/continuation/commit", map[string]any{
		"task_id": "t-none",
		"index":   0,
		"title":   "x",
		"content": "y",
		"target":  "new_chapter",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("commit no task status = %d body=%s", resp.Code, resp.Body.String())
	}

	// 非法目标
	application.SetContinuationGeneratorFactoryForTest(continuationStubFactory(nil))
	exploreResp := performJSONRequest(t, server, http.MethodPost, "/api/continuation/explore", map[string]any{
		"workspace": workspace,
		"depth":     "short",
		"mode":      "direction",
	})
	events := readSSEEvents(t, exploreResp.Body.String())
	var taskID string
	for _, event := range events {
		if event["event"] == "explore_start" {
			taskID, _ = event["task_id"].(string)
		}
	}
	if taskID == "" {
		t.Fatalf("no task created")
	}
	resp = performJSONRequest(t, server, http.MethodPost, "/api/continuation/commit", map[string]any{
		"task_id": taskID,
		"index":   0,
		"title":   "x",
		"content": "y",
		"target":  "bogus",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("invalid target status = %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestContinuationTaskPersistenceAcrossRestart(t *testing.T) {
	root := t.TempDir()
	service := continuation.NewService(root)
	task, err := service.Create(continuation.ExploreRequest{Workspace: root, Depth: "medium", Mode: "excerpt"}, func() {})
	if err != nil {
		t.Fatal(err)
	}
	task.UpdateCandidate(continuation.Candidate{Index: 0, Title: "t", Direction: "d", Excerpt: "e", Status: string(continuation.CandidateDone)})
	task.SetStatus(continuation.TaskStatusDone)

	// 模拟重启：新建实例从磁盘恢复
	restored, err := continuation.NewService(root).Get(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	status, candidates := restored.StatusNow()
	if status != continuation.TaskStatusDone || len(candidates) != 1 {
		t.Fatalf("restored status=%s candidates=%v", status, candidates)
	}
	if _, err := os.Stat(filepath.Join(root, ".casemagica", "continuations", task.ID, "meta.json")); err != nil {
		t.Fatalf("meta.json missing: %v", err)
	}
	_ = bytes.NewReader
	_ = time.Now
}
