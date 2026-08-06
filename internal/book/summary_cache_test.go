package book

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSummaryCacheHitsAndInvalidates(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	chapter := filepath.Join(root, "chapters", "ch00001-第一章.md")
	if err := os.WriteFile(chapter, []byte("第一章 开局\n\n天亮了。"), 0o644); err != nil {
		t.Fatal(err)
	}

	service := NewService(root)
	first, err := service.Summary()
	if err != nil {
		t.Fatal(err)
	}
	if first.ChapterCount != 1 {
		t.Fatalf("chapter count = %d", first.ChapterCount)
	}

	// 文件未变化：缓存命中（通过返回同一 WorkspaceSummary 值验证内容一致）
	second, err := service.Summary()
	if err != nil {
		t.Fatal(err)
	}
	if second.ChapterCount != 1 || second.Chapters[0].Words != first.Chapters[0].Words {
		t.Fatalf("cache mismatch: %#v vs %#v", second, first)
	}

	// 修改章节文件（保证 mtime 前进）：缓存应失效并反映新内容
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(chapter, []byte("第一章 开局\n\n天亮了。\n\n林川醒来。"), 0o644); err != nil {
		t.Fatal(err)
	}
	third, err := service.Summary()
	if err != nil {
		t.Fatal(err)
	}
	if third.ChapterCount != 1 || third.Chapters[0].Words <= first.Chapters[0].Words {
		t.Fatalf("cache should invalidate after file change: %#v", third.Chapters[0])
	}
}

func TestSummaryCacheInvalidatesOnNewChapter(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	service := NewService(root)
	if _, err := service.Summary(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "chapters", "ch00001-第一章.md"), []byte("内容"), 0o644); err != nil {
		t.Fatal(err)
	}
	summary, err := service.Summary()
	if err != nil {
		t.Fatal(err)
	}
	if summary.ChapterCount != 1 {
		t.Fatalf("new chapter should invalidate cache, count = %d", summary.ChapterCount)
	}
}

func TestTreeCacheHitsAndInvalidates(t *testing.T) {
	root := t.TempDir()
	service := NewService(root)
	first, err := service.Tree()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ideas.md"), []byte("# 灵感"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := service.Tree()
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != len(first)+1 {
		t.Fatalf("tree cache should invalidate after new file: %d -> %d", len(first), len(second))
	}
}

func TestSummaryCacheIgnoresHiddenDirectoryChanges(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "chapters", "ch00001-第一章.md"), []byte("内容"), 0o644); err != nil {
		t.Fatal(err)
	}
	service := NewService(root)
	first, err := service.Summary()
	if err != nil {
		t.Fatal(err)
	}
	// .casemagica 私有目录的会话/账本变化不应触发重建
	if err := os.MkdirAll(filepath.Join(root, ".casemagica", "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".casemagica", "sessions", "x.jsonl"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := service.Summary()
	if err != nil {
		t.Fatal(err)
	}
	if second.ChapterCount != first.ChapterCount {
		t.Fatalf("hidden dir changes should not invalidate cache: %#v vs %#v", second, first)
	}
}
