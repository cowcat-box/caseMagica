package book

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIdentifySettingsKindByFilename(t *testing.T) {
	cases := []struct {
		filename string
		want     SettingsKind
	}{
		{"outline.md", SettingsKindOutline},
		{"CREATOR.md", SettingsKindRules},
		{"creator.md", SettingsKindRules},
		{"progress.md", SettingsKindProgress},
		{"character-states.md", SettingsKindCharacterStates},
		{"ideas.md", SettingsKindIdeas},
		{"group3-第三组.md", SettingsKindChapterGroup},
		{"group1.md", SettingsKindChapterGroup},
		{"unknown.md", SettingsKindUnknown},
	}
	for _, tc := range cases {
		if got := IdentifySettingsKind(tc.filename, ""); got != tc.want {
			t.Errorf("IdentifySettingsKind(%q) = %q, want %q", tc.filename, got, tc.want)
		}
	}
}

func TestIdentifySettingsKindByContentHeuristics(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    SettingsKind
	}{
		{"大纲标题", "# 主角身份设定大纲\n\n主线推进。", SettingsKindOutline},
		{"创作规则", "# 创作规则\n\n不剧透。", SettingsKindRules},
		{"细纲标题", "# 第一卷细纲\n\n三章。", SettingsKindChapterGroup},
		{"章节组", "## 章节组二\n\n推进。", SettingsKindChapterGroup},
		{"角色状态", "# 角色状态\n\n林川：健康。", SettingsKindCharacterStates},
		{"无识别", "随便的文本\n\n没有关键词。", SettingsKindUnknown},
	}
	for _, tc := range cases {
		if got := IdentifySettingsKind("whatever.txt", tc.content); got != tc.want {
			t.Errorf("%s IdentifySettingsKind = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestImportSettingsFileBacksUpExistingTarget(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "setting"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "setting", "outline.md"), []byte("旧大纲"), 0o644); err != nil {
		t.Fatal(err)
	}
	service := NewService(root)
	result, err := service.ImportSettingsFile([]byte("新大纲内容"), SettingsKindOutline)
	if err != nil {
		t.Fatal(err)
	}
	if result.TargetPath != "setting/outline.md" {
		t.Fatalf("target = %q", result.TargetPath)
	}
	if result.BackupPath == "" || !strings.Contains(result.BackupPath, "backups/settings-import") {
		t.Fatalf("backup = %q", result.BackupPath)
	}
	content, err := service.ReadFile("setting/outline.md")
	if err != nil {
		t.Fatal(err)
	}
	if content != "新大纲内容" {
		t.Fatalf("content = %q", content)
	}
	backupAbs := filepath.Join(root, filepath.FromSlash(result.BackupPath))
	backup, err := os.ReadFile(backupAbs)
	if err != nil {
		t.Fatalf("读取备份失败: %v", err)
	}
	if string(backup) != "旧大纲" {
		t.Fatalf("backup content = %q", backup)
	}
}

func TestImportSettingsFileChapterGroupAutoNumbering(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "setting", "chapter-groups"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"group1-第一组.md", "group3-第三组.md"} {
		if err := os.WriteFile(filepath.Join(root, "setting", "chapter-groups", name), []byte("细纲"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	service := NewService(root)
	result, err := service.ImportSettingsFile([]byte("新细纲"), SettingsKindChapterGroup)
	if err != nil {
		t.Fatal(err)
	}
	if result.TargetPath != "setting/chapter-groups/group4.md" {
		t.Fatalf("target = %q, want group4", result.TargetPath)
	}
	content, err := service.ReadFile(result.TargetPath)
	if err != nil {
		t.Fatal(err)
	}
	if content != "新细纲" {
		t.Fatalf("content = %q", content)
	}
}

func TestImportSettingsFileUnknownKind(t *testing.T) {
	root := t.TempDir()
	_, err := NewService(root).ImportSettingsFile([]byte("x"), SettingsKindUnknown)
	if !errors.Is(err, ErrSettingsImportTypeRequired) {
		t.Fatalf("err = %v, want ErrSettingsImportTypeRequired", err)
	}
}

func TestSettingsExportSingleWhitelist(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "setting"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "setting", "outline.md"), []byte("大纲内容"), 0o644); err != nil {
		t.Fatal(err)
	}
	service := NewService(root)
	filename, content, err := service.SettingsExportSingle("setting/outline.md")
	if err != nil {
		t.Fatal(err)
	}
	if filename != "outline.md" || string(content) != "大纲内容" {
		t.Fatalf("filename=%q content=%q", filename, content)
	}
	if _, _, err := service.SettingsExportSingle("book.json"); err == nil {
		t.Fatalf("book.json should not be exportable via settings single export")
	}
	if _, _, err := service.SettingsExportSingle("../secret.md"); err == nil {
		t.Fatalf("path traversal should be rejected")
	}
}

func TestSettingsExportGroupsZipsAllGroups(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "setting", "chapter-groups"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"group1.md", "group2.md"} {
		if err := os.WriteFile(filepath.Join(root, "setting", "chapter-groups", name), []byte("细纲"+name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	service := NewService(root)
	data, err := service.SettingsExportGroups()
	if err != nil {
		t.Fatal(err)
	}
	entries := zipEntries(t, data)
	if entries["setting/chapter-groups/group1.md"] != "细纲group1.md" {
		t.Fatalf("entries = %v", entries)
	}
	if entries["setting/chapter-groups/group2.md"] != "细纲group2.md" {
		t.Fatalf("entries = %v", entries)
	}
}

func TestSettingsImportExportRoundTrip(t *testing.T) {
	root := t.TempDir()
	service := NewService(root)
	if _, err := service.ImportSettingsFile([]byte("# 新大纲\n\n内容。"), SettingsKindOutline); err != nil {
		t.Fatal(err)
	}
	_, content, err := service.SettingsExportSingle("setting/outline.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "# 新大纲\n\n内容。" {
		t.Fatalf("round-trip content = %q", content)
	}
}
