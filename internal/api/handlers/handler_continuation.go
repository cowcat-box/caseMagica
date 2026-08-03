package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"casemagica/internal/continuation"
)

// continuationSSEHandler 把推演事件写入 SSE 流。
type continuationSSEHandler struct {
	writer io.Writer
	taskID string
}

func (h *continuationSSEHandler) OnCandidateStart(task *continuation.Task, index int) {
	_ = h.write("candidate_start", map[string]any{"task_id": h.taskID, "index": index})
}

func (h *continuationSSEHandler) OnCandidateDelta(task *continuation.Task, index int, content string) {
	_ = h.write("candidate_delta", map[string]any{"task_id": h.taskID, "index": index, "content": content})
}

func (h *continuationSSEHandler) OnCandidateDone(task *continuation.Task, candidate continuation.Candidate) {
	_ = h.write("candidate_done", map[string]any{"task_id": h.taskID, "index": candidate.Index, "candidate": candidate})
}

func (h *continuationSSEHandler) OnCandidateFailed(task *continuation.Task, index int, message string) {
	_ = h.write("candidate_failed", map[string]any{"task_id": h.taskID, "index": index, "error": message})
}

func (h *continuationSSEHandler) OnExploreDone(task *continuation.Task) {
	status, _ := task.StatusNow()
	_ = h.write("explore_done", map[string]any{"task_id": h.taskID, "status": status})
}

func (h *continuationSSEHandler) write(eventType string, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(h.writer, "event: %s\ndata: %s\n\n", eventType, payload)
	return err
}

// HandleContinuationExplore POST /api/continuation/explore — 发起续写推演（SSE 流式）。
func (h *Handlers) HandleContinuationExplore(ctx context.Context, c *app.RequestContext) {
	var request continuation.ExploreRequest
	if err := c.BindJSON(&request); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequest")
		return
	}
	if strings.TrimSpace(request.Workspace) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.books.pathRequired")
		return
	}
	task, err := h.app.ContinuationExplore(request)
	if err != nil {
		status := consts.StatusBadRequest
		if err == continuation.ErrTaskInProgress {
			status = consts.StatusConflict
		}
		writeError(c, status, err.Error())
		return
	}
	log.Printf("[api] 续写推演 begin task=%s workspace=%s depth=%s mode=%s anchor=%q", task.ID, request.Workspace, request.Depth, request.Mode, request.AnchorChapter)

	c.Response.Header.Set("Content-Type", "text/event-stream")
	c.Response.Header.Set("Cache-Control", "no-cache")
	c.Response.Header.Set("Connection", "keep-alive")
	c.Response.ImmediateHeaderFlush = true

	pr, pw := io.Pipe()
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("[api] 续写推演 panic recovered task=%s err=%v", task.ID, recovered)
			}
			_ = pw.Close()
		}()
		handler := &continuationSSEHandler{writer: pw, taskID: task.ID}
		_ = handler.write("explore_start", map[string]any{"task_id": task.ID})
		status := h.app.ContinuationRunGeneration(task, handler)
		_ = handler.write("explore_done", map[string]any{"task_id": task.ID, "status": status})
	}()
	c.Response.SetBodyStream(pr, -1)
}

// HandleContinuationTasks GET /api/continuation/tasks?workspace=... — 列出推演任务。
func (h *Handlers) HandleContinuationTasks(ctx context.Context, c *app.RequestContext) {
	workspace := strings.TrimSpace(c.Query("workspace"))
	if workspace == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.books.pathRequired")
		return
	}
	tasks, err := h.app.ContinuationTaskList(workspace)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"tasks": tasks})
}

// HandleContinuationTaskStream GET /api/continuation/tasks/:id/stream — 恢复任务当前状态快照。
func (h *Handlers) HandleContinuationTaskStream(ctx context.Context, c *app.RequestContext) {
	taskID := strings.TrimSpace(c.Param("id"))
	task, err := h.app.ContinuationTask(taskID)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}

	c.Response.Header.Set("Content-Type", "text/event-stream")
	c.Response.Header.Set("Cache-Control", "no-cache")
	c.Response.Header.Set("Connection", "keep-alive")
	c.Response.ImmediateHeaderFlush = true

	pr, pw := io.Pipe()
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("[api] 续写推演恢复 panic recovered task=%s err=%v", taskID, recovered)
			}
			_ = pw.Close()
		}()
		handler := &continuationSSEHandler{writer: pw, taskID: task.ID}
		status, candidates := task.StatusNow()
		for _, candidate := range candidates {
			switch candidate.Status {
			case string(continuation.CandidateDone):
				handler.OnCandidateDone(task, candidate)
			case string(continuation.CandidateFailed):
				handler.OnCandidateFailed(task, candidate.Index, candidate.Error)
			default:
				handler.OnCandidateStart(task, candidate.Index)
			}
		}
		_ = handler.write("explore_done", map[string]any{"task_id": task.ID, "status": status})
	}()
	c.Response.SetBodyStream(pr, -1)
}

// HandleContinuationTaskAbort POST /api/continuation/tasks/:id/abort — 中止推演任务。
func (h *Handlers) HandleContinuationTaskAbort(ctx context.Context, c *app.RequestContext) {
	taskID := strings.TrimSpace(c.Param("id"))
	if taskID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequest")
		return
	}
	if err := h.app.ContinuationAbortTask(taskID); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "cancelled"})
}

// HandleContinuationTaskDelete DELETE /api/continuation/tasks/:id — 删除任务记录（备份）。
func (h *Handlers) HandleContinuationTaskDelete(ctx context.Context, c *app.RequestContext) {
	taskID := strings.TrimSpace(c.Param("id"))
	if taskID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequest")
		return
	}
	backupPath, err := h.app.ContinuationDeleteTask(taskID)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"backup_path": backupPath})
}

// HandleContinuationCommit POST /api/continuation/commit — 提交候选到章节。
func (h *Handlers) HandleContinuationCommit(ctx context.Context, c *app.RequestContext) {
	var request continuation.CommitRequest
	if err := c.BindJSON(&request); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequest")
		return
	}
	if strings.TrimSpace(request.TaskID) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequest")
		return
	}
	if request.Target != string(continuation.TargetNewChapter) && request.Target != string(continuation.TargetAppend) {
		writeErrorKey(c, consts.StatusBadRequest, "api.continuation.invalidTarget")
		return
	}
	if strings.TrimSpace(request.Content) == "" && request.Target == string(continuation.TargetAppend) {
		writeErrorKey(c, consts.StatusBadRequest, "api.continuation.emptyContent")
		return
	}
	result, err := h.app.ContinuationCommit(request)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, result)
}
