package handlers

import (
	"context"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	novaskills "casemagica/internal/skills"
)

// HandleSkillDraftCreate POST /api/skills/drafts — 创建 Skill 草稿（供前端直接创建兜底）。
func (h *Handlers) HandleSkillDraftCreate(ctx context.Context, c *app.RequestContext) {
	var req struct {
		Name        string            `json:"name"`
		Description string            `json:"description"`
		Agent       string            `json:"agent,omitempty"`
		Context     string            `json:"context,omitempty"`
		Model       string            `json:"model,omitempty"`
		Body        string            `json:"body"`
		Files       map[string]string `json:"files,omitempty"`
	}
	if err := c.BindJSON(&req); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequest")
		return
	}
	meta, err := h.app.SkillDraftCreate(novaskills.CreateDraftInput{
		Name:        req.Name,
		Description: req.Description,
		Agent:       req.Agent,
		Context:     req.Context,
		Model:       req.Model,
		Body:        req.Body,
		Files:       req.Files,
		SourceScope: "api",
	})
	if err != nil {
		status := consts.StatusBadRequest
		if strings.Contains(err.Error(), "already exists") {
			status = consts.StatusConflict
		}
		writeError(c, status, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, meta)
}

// HandleSkillDraftList GET /api/skills/drafts — 列出全部 Skill 草稿。
func (h *Handlers) HandleSkillDraftList(ctx context.Context, c *app.RequestContext) {
	drafts, err := h.app.SkillDraftList()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"drafts": drafts})
}

// HandleSkillDraftRead GET /api/skills/drafts/:name — 读取草稿完整内容。
func (h *Handlers) HandleSkillDraftRead(ctx context.Context, c *app.RequestContext) {
	name := strings.TrimSpace(c.Param("name"))
	if name == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.skills.scopeNameRequired")
		return
	}
	doc, err := h.app.SkillDraftRead(name)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, doc)
}

// HandleSkillDraftUpdate PUT /api/skills/drafts/:name — 更新草稿正文。
func (h *Handlers) HandleSkillDraftUpdate(ctx context.Context, c *app.RequestContext) {
	name := strings.TrimSpace(c.Param("name"))
	if name == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.skills.scopeNameRequired")
		return
	}
	var req struct {
		Description string `json:"description"`
		Agent       string `json:"agent,omitempty"`
		Context     string `json:"context,omitempty"`
		Model       string `json:"model,omitempty"`
		Body        string `json:"body"`
	}
	if err := c.BindJSON(&req); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequest")
		return
	}
	meta, err := h.app.SkillDraftUpdate(name, req.Description, req.Agent, req.Context, req.Model, req.Body)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, meta)
}

// HandleSkillDraftDiscard DELETE /api/skills/drafts/:name — 丢弃草稿（移至备份区）。
func (h *Handlers) HandleSkillDraftDiscard(ctx context.Context, c *app.RequestContext) {
	name := strings.TrimSpace(c.Param("name"))
	if name == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.skills.scopeNameRequired")
		return
	}
	backupPath, err := h.app.SkillDraftDiscard(name)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"backup_path": backupPath})
}

// HandleSkillDraftConfirm POST /api/skills/drafts/:name/confirm — 确认导入草稿为正式 Skill。
func (h *Handlers) HandleSkillDraftConfirm(ctx context.Context, c *app.RequestContext) {
	name := strings.TrimSpace(c.Param("name"))
	if name == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.skills.scopeNameRequired")
		return
	}
	var req struct {
		Scope novaskills.Scope `json:"scope"`
	}
	if err := c.BindJSON(&req); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequest")
		return
	}
	if req.Scope != novaskills.ScopeUser && req.Scope != novaskills.ScopeWorkspace {
		writeErrorKey(c, consts.StatusBadRequest, "api.skills.invalidScope")
		return
	}
	doc, err := h.app.SkillDraftConfirm(ctx, name, req.Scope)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, doc)
}
