package skills

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestDraftStore(t *testing.T) *DraftStore {
	t.Helper()
	return NewDraftStore(t.TempDir())
}

func TestDraftCreateReadUpdateDiscard(t *testing.T) {
	store := newTestDraftStore(t)
	meta, err := store.CreateDraft(CreateDraftInput{
		Name:        "my-skill",
		Description: "测试 Skill",
		Agent:       "ide",
		Body:        "## 工作流\n\n1. 读取大纲\n2. 写作",
		Files:       map[string]string{"templates/example.md": "模板内容"},
		SourceScope: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if meta.Name != "my-skill" || len(meta.Files) != 1 {
		t.Fatalf("meta mismatch: %#v", meta)
	}

	doc, err := store.ReadDraft("my-skill")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.Content, "name: \"my-skill\"") || !strings.Contains(doc.Content, "## 工作流") {
		t.Fatalf("content mismatch:\n%s", doc.Content)
	}
	if doc.Files["templates/example.md"] != "模板内容" {
		t.Fatalf("extra file mismatch: %v", doc.Files)
	}
	if _, err := os.Stat(filepath.Join(store.draftDir("my-skill"), "SKILL.md")); err != nil {
		t.Fatalf("SKILL.md not written: %v", err)
	}

	updated, err := store.UpdateDraftBody("my-skill", "新描述", "ide, config_manager", "", "", "新正文")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Description != "新描述" || updated.Agent != "ide, config_manager" {
		t.Fatalf("update mismatch: %#v", updated)
	}
	doc, err = store.ReadDraft("my-skill")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.Content, "新正文") || !strings.Contains(doc.Content, "agent: \"ide, config_manager\"") {
		t.Fatalf("updated content mismatch:\n%s", doc.Content)
	}

	drafts, err := store.ListDrafts()
	if err != nil {
		t.Fatal(err)
	}
	if len(drafts) != 1 || drafts[0].Name != "my-skill" {
		t.Fatalf("list mismatch: %#v", drafts)
	}

	backup, err := store.DiscardDraft("my-skill")
	if err != nil {
		t.Fatal(err)
	}
	if backup == "" || !strings.Contains(backup, "backups") {
		t.Fatalf("backup = %q", backup)
	}
	if _, err := os.Stat(filepath.Join(store.Root(), "my-skill")); !os.IsNotExist(err) {
		t.Fatalf("draft dir should be removed after discard")
	}
	if _, err := store.ReadDraft("my-skill"); !errors.Is(err, ErrDraftNotFound) {
		t.Fatalf("err = %v, want ErrDraftNotFound", err)
	}
}

func TestDraftNameValidation(t *testing.T) {
	store := newTestDraftStore(t)
	for _, name := range []string{"Bad Name", "../escape", "带中文", ""} {
		if _, err := store.CreateDraft(CreateDraftInput{Name: name, Description: "x", Body: "y"}); err == nil {
			t.Errorf("CreateDraft(%q) should fail", name)
		}
	}
}

func TestDraftRejectsInvalidExtraFilePaths(t *testing.T) {
	store := newTestDraftStore(t)
	for _, rel := range []string{"../escape.md", "a/b/../../up.md", "SKILL.md"} {
		_, err := store.CreateDraft(CreateDraftInput{
			Name:        "valid-name",
			Description: "x",
			Body:        "y",
			Files:       map[string]string{rel: "content"},
		})
		if err == nil {
			t.Errorf("CreateDraft with file %q should fail", rel)
		}
		if _, listErr := store.ListDrafts(); listErr != nil {
			t.Fatalf("list after failed create: %v", listErr)
		}
		if _, readErr := store.ReadDraft("valid-name"); !errors.Is(readErr, ErrDraftNotFound) {
			t.Errorf("failed create should not leave draft behind for file %q", rel)
		}
	}
}

func TestDraftDuplicateNameRejected(t *testing.T) {
	store := newTestDraftStore(t)
	if _, err := store.CreateDraft(CreateDraftInput{Name: "dup", Description: "x", Body: "y"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateDraft(CreateDraftInput{Name: "dup", Description: "z", Body: "w"}); !errors.Is(err, ErrDraftExists) {
		t.Fatalf("err = %v, want ErrDraftExists", err)
	}
}

func TestDraftConfirmCreatesSkillAndRemovesDraft(t *testing.T) {
	store := newTestDraftStore(t)
	denovaDir := filepath.Dir(store.Root())
	if _, err := store.CreateDraft(CreateDraftInput{
		Name:        "confirmed-skill",
		Description: "确认导入的 Skill",
		Body:        "## 使用\n\n完成某项任务。",
	}); err != nil {
		t.Fatal(err)
	}
	dirs := NewDirectories("", denovaDir, t.TempDir())
	doc, err := store.ConfirmDraft("confirmed-skill", ScopeUser, dirs)
	if err != nil {
		t.Fatal(err)
	}
	if doc.SkillSummary.Name != "confirmed-skill" {
		t.Fatalf("skill name = %q", doc.SkillSummary.Name)
	}
	skillPath := filepath.Join(denovaDir, "skills", "confirmed-skill", SkillFileName)
	data, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("confirmed skill missing: %v", err)
	}
	if !strings.Contains(string(data), "确认导入的 Skill") {
		t.Fatalf("confirmed skill content mismatch:\n%s", data)
	}
	if _, err := store.ReadDraft("confirmed-skill"); !errors.Is(err, ErrDraftNotFound) {
		t.Fatalf("draft should be removed after confirm, err = %v", err)
	}
}

func TestDraftConfirmPreservesExtraFiles(t *testing.T) {
	store := newTestDraftStore(t)
	denovaDir := filepath.Dir(store.Root())
	if _, err := store.CreateDraft(CreateDraftInput{
		Name:        "with-files",
		Description: "带附加文件",
		Body:        "正文",
		Files:       map[string]string{"data/template.txt": "模板数据"},
	}); err != nil {
		t.Fatal(err)
	}
	dirs := NewDirectories("", denovaDir, t.TempDir())
	if _, err := store.ConfirmDraft("with-files", ScopeUser, dirs); err != nil {
		t.Fatal(err)
	}
	extra, err := os.ReadFile(filepath.Join(denovaDir, "skills", "with-files", "data", "template.txt"))
	if err != nil {
		t.Fatalf("extra file missing after confirm: %v", err)
	}
	if string(extra) != "模板数据" {
		t.Fatalf("extra file content = %q", extra)
	}
}

func TestDraftConfirmConflictWithExistingSkill(t *testing.T) {
	store := newTestDraftStore(t)
	denovaDir := filepath.Dir(store.Root())
	if _, err := store.CreateDraft(CreateDraftInput{
		Name:        "conflict-skill",
		Description: "草稿",
		Body:        "正文",
	}); err != nil {
		t.Fatal(err)
	}
	dirs := NewDirectories("", denovaDir, t.TempDir())
	if _, err := ConfirmDraftWithExisting(t, store, dirs); err == nil {
		t.Fatalf("confirm with existing skill should fail")
	}
}

func ConfirmDraftWithExisting(t *testing.T, store *DraftStore, dirs []Directory) (Document, error) {
	t.Helper()
	// 先通过 CreateDocument 建立同名正式 Skill，再尝试确认草稿
	if _, err := CreateDocument(context.Background(), dirs, ScopeUser, "conflict-skill", "已存在"); err != nil {
		t.Fatal(err)
	}
	return store.ConfirmDraft("conflict-skill", ScopeUser, dirs)
}
