package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"casemagica/internal/skills"
)

func TestSkillDraftCRUDAndConfirmAPI(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")

	createResp := performJSONRequest(t, server, http.MethodPost, "/api/skills/drafts", map[string]any{
		"name":        "draft-skill",
		"description": "草稿测试",
		"agent":       "ide",
		"body":        "## 工作流\n\n完成测试。",
		"files":       map[string]string{"data/ref.txt": "参考资料"},
	})
	if createResp.Code != http.StatusOK {
		t.Fatalf("create status = %d body=%s", createResp.Code, createResp.Body.String())
	}

	listResp := performJSONRequest(t, server, http.MethodGet, "/api/skills/drafts", nil)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", listResp.Code, listResp.Body.String())
	}
	var listBody struct {
		Drafts []skills.DraftMeta `json:"drafts"`
	}
	decodeResponse(t, listResp.Body.Bytes(), &listBody)
	if len(listBody.Drafts) != 1 || listBody.Drafts[0].Name != "draft-skill" {
		t.Fatalf("list mismatch: %#v", listBody.Drafts)
	}

	readResp := performJSONRequest(t, server, http.MethodGet, "/api/skills/drafts/draft-skill", nil)
	if readResp.Code != http.StatusOK {
		t.Fatalf("read status = %d body=%s", readResp.Code, readResp.Body.String())
	}
	var readBody skills.DraftDocument
	decodeResponse(t, readResp.Body.Bytes(), &readBody)
	if !strings.Contains(readBody.Content, "完成测试") || readBody.Files["data/ref.txt"] != "参考资料" {
		t.Fatalf("read mismatch: %#v", readBody)
	}

	confirmResp := performJSONRequest(t, server, http.MethodPost, "/api/skills/drafts/draft-skill/confirm", map[string]string{"scope": "user"})
	if confirmResp.Code != http.StatusOK {
		t.Fatalf("confirm status = %d body=%s", confirmResp.Code, confirmResp.Body.String())
	}

	skillPath := filepath.Join(application.Workspace(), "skills", "draft-skill", skills.SkillFileName)
	data, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("confirmed skill missing: %v", err)
	}
	if !strings.Contains(string(data), "完成测试") {
		t.Fatalf("confirmed skill content mismatch:\n%s", data)
	}
	extra, err := os.ReadFile(filepath.Join(application.Workspace(), "skills", "draft-skill", "data", "ref.txt"))
	if err != nil || string(extra) != "参考资料" {
		t.Fatalf("confirmed extra file missing: %v %q", err, extra)
	}

	listResp = performJSONRequest(t, server, http.MethodGet, "/api/skills/drafts", nil)
	decodeResponse(t, listResp.Body.Bytes(), &listBody)
	if len(listBody.Drafts) != 0 {
		t.Fatalf("draft should be removed after confirm, got %#v", listBody.Drafts)
	}
}

func TestSkillDraftDiscardMovesToBackupAPI(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")

	performJSONRequest(t, server, http.MethodPost, "/api/skills/drafts", map[string]any{
		"name":        "discard-me",
		"description": "丢弃测试",
		"body":        "正文",
	})
	discardResp := performJSONRequest(t, server, http.MethodDelete, "/api/skills/drafts/discard-me", nil)
	if discardResp.Code != http.StatusOK {
		t.Fatalf("discard status = %d body=%s", discardResp.Code, discardResp.Body.String())
	}
	var result struct {
		BackupPath string `json:"backup_path"`
	}
	decodeResponse(t, discardResp.Body.Bytes(), &result)
	if !strings.Contains(result.BackupPath, "backups") {
		t.Fatalf("backup path = %q", result.BackupPath)
	}
	if _, err := os.Stat(filepath.Join(application.Workspace(), "skill-drafts", "discard-me")); !os.IsNotExist(err) {
		t.Fatalf("draft dir should be removed after discard")
	}
	if _, err := os.Stat(result.BackupPath); err != nil {
		t.Fatalf("backup missing: %v", err)
	}
}

func TestSkillDraftConfirmRejectsExistingSkillAPI(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")

	createResp := performJSONRequest(t, server, http.MethodPost, "/api/skills", map[string]any{
		"scope": "user", "name": "dup-skill", "description": "已存在", "agents": []string{},
	})
	if createResp.Code != http.StatusOK {
		t.Fatalf("create skill status = %d body=%s", createResp.Code, createResp.Body.String())
	}
	performJSONRequest(t, server, http.MethodPost, "/api/skills/drafts", map[string]any{
		"name": "dup-skill", "description": "草稿", "body": "正文",
	})
	confirmResp := performJSONRequest(t, server, http.MethodPost, "/api/skills/drafts/dup-skill/confirm", map[string]string{"scope": "user"})
	if confirmResp.Code != http.StatusBadRequest {
		t.Fatalf("confirm with existing skill status = %d body=%s", confirmResp.Code, confirmResp.Body.String())
	}
}
