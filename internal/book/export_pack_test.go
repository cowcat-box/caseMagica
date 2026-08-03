package book

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// newExportPackWorkspace 构建一个含分卷章节、细纲与设定的测试工作区。
func newExportPackWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	chapterDir := filepath.Join(root, "chapters", "v00001-第一卷-风起")
	if err := os.MkdirAll(chapterDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "setting", "chapter-groups"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"chapters/v00001-第一卷-风起/ch00001-第一章-开局.md": "第一章 开局\n\n天亮了。",
		"chapters/v00001-第一卷-风起/ch00002-第二章-追光.md": "# 第二章 追光\n\n林川踏入雨夜。",
		"chapters/v00001-第一卷-风起/ch00003-第三章-空章.md": "",
		"setting/chapter-groups/group1-第一组.md":        "# 第一组\n\n前五章推进。",
		"setting/chapter-groups/group2-第二组.md":        "# 第二组\n\n后五章收束。",
		"setting/outline.md":                            "# 书名大纲",
		"CREATOR.md":                                    "# 创作规则",
		"setting/progress.md":                           "# 进度",
		"setting/character-states.md":                   "# 角色状态",
		"ideas.md":                                      "# 灵感",
		"book.json":                                     `{"title":"星河边境","author":"CaseMagica"}`,
	}
	for path, content := range files {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(path)), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func zipEntries(t *testing.T, data []byte) map[string]string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string]string{}
	for _, file := range reader.File {
		rc, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		entries[file.Name] = string(content)
	}
	return entries
}

func TestBuildExportPackKeepsOriginalDirectoryStructure(t *testing.T) {
	root := newExportPackWorkspace(t)
	pack, err := NewService(root).BuildExportPack(ExportSelection{
		Chapters: true,
		Outline:  true,
		Rules:    true,
		Meta:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]bool{}
	for _, file := range pack.Files {
		paths[file.Path] = true
	}
	wantPaths := []string{
		"chapters/v00001-第一卷-风起/ch00001-第一章-开局.md",
		"chapters/v00001-第一卷-风起/ch00002-第二章-追光.md",
		"setting/outline.md",
		"CREATOR.md",
		"book.json",
	}
	for _, path := range wantPaths {
		if !paths[path] {
			t.Errorf("export pack missing %q, got %v", path, paths)
		}
	}
	if paths["chapters/v00001-第一卷-风起/ch00003-第三章-空章.md"] {
		t.Errorf("empty chapter should be skipped")
	}
}

func TestBuildExportPackChapterGroupsLatest(t *testing.T) {
	root := newExportPackWorkspace(t)
	pack, err := NewService(root).BuildExportPack(ExportSelection{ChapterGroups: ChapterGroupsLatest})
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Files) != 1 {
		t.Fatalf("files = %d, want 1", len(pack.Files))
	}
	if pack.Files[0].Path != "setting/chapter-groups/group2-第二组.md" {
		t.Fatalf("latest group = %q", pack.Files[0].Path)
	}
}

func TestBuildExportPackChapterGroupsAll(t *testing.T) {
	root := newExportPackWorkspace(t)
	pack, err := NewService(root).BuildExportPack(ExportSelection{ChapterGroups: ChapterGroupsAll})
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Files) != 2 {
		t.Fatalf("files = %d, want 2", len(pack.Files))
	}
}

func TestBuildExportPackMissingSettingFilesSkipped(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "chapters", "ch00001-第一章.md"), []byte("内容"), 0o644); err != nil {
		t.Fatal(err)
	}
	pack, err := NewService(root).BuildExportPack(ExportSelection{
		Chapters: true,
		Outline:  true,
		Rules:    true,
		Progress: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Files) != 1 || pack.Files[0].Path != "chapters/ch00001-第一章.md" {
		t.Fatalf("files = %v, want only the chapter", pack.Files)
	}
}

func TestBuildExportPackEmptySelection(t *testing.T) {
	root := newExportPackWorkspace(t)
	_, err := NewService(root).BuildExportPack(ExportSelection{})
	if !errors.Is(err, ErrNoExportableContent) {
		t.Fatalf("err = %v, want ErrNoExportableContent", err)
	}
}

func TestBuildExportPackAllContentMissing(t *testing.T) {
	root := t.TempDir()
	_, err := NewService(root).BuildExportPack(ExportSelection{Chapters: true, Outline: true})
	if !errors.Is(err, ErrNoExportableContent) {
		t.Fatalf("err = %v, want ErrNoExportableContent", err)
	}
}

func TestBuildExportZipPreservesStructureAndUTF8Names(t *testing.T) {
	root := newExportPackWorkspace(t)
	pack, err := NewService(root).BuildExportPack(ExportSelection{Chapters: true})
	if err != nil {
		t.Fatal(err)
	}
	data, err := BuildExportZip(pack.Files)
	if err != nil {
		t.Fatal(err)
	}
	entries := zipEntries(t, data)
	want := map[string]string{
		"chapters/v00001-第一卷-风起/ch00001-第一章-开局.md": "第一章 开局\n\n天亮了。",
		"chapters/v00001-第一卷-风起/ch00002-第二章-追光.md": "# 第二章 追光\n\n林川踏入雨夜。",
	}
	for path, content := range want {
		if entries[path] != content {
			t.Errorf("zip entry %q = %q, want %q", path, entries[path], content)
		}
	}
}
