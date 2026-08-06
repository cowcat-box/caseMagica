package book

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"casemagica/internal/workspacepath"
)

// 包级扫描缓存：Summary 与 Tree 的全量目录扫描按"文件 mtime+size 指纹"失效。
// 指纹遍历只做 stat、不读文件内容（138 章节工作区实测 ~1ms），命中时直接返回
// 上次结果，避免每 3 秒轮询/每次 Agent 变更/每次保存都全量读取所有章节文件。
// 指纹与 BuildFileTree 一致地跳过隐藏文件与隐藏目录（.casemagica 的频繁变化不触发重建）。

var (
	summaryCacheMu sync.Mutex
	summaryCache   = map[string]*summaryCacheEntry{}

	treeCacheMu sync.Mutex
	treeCache   = map[string]*treeCacheEntry{}
)

type summaryCacheEntry struct {
	fingerprint string
	summary     WorkspaceSummary
	err         error
}

type treeCacheEntry struct {
	fingerprint string
	tree        []*FileNode
	err         error
}

// Summary 带缓存的章节统计汇总；指纹未变化时复用上次结果。
func (s *Service) Summary() (WorkspaceSummary, error) {
	fingerprint := summaryFingerprint(s.workspace)
	summaryCacheMu.Lock()
	defer summaryCacheMu.Unlock()
	if entry, ok := summaryCache[s.workspace]; ok && entry.fingerprint == fingerprint {
		return entry.summary, entry.err
	}
	summary, err := s.summaryUncached()
	summaryCache[s.workspace] = &summaryCacheEntry{fingerprint: fingerprint, summary: summary, err: err}
	return summary, err
}

// Tree 带缓存的文件树；指纹未变化时复用上次结果。
func (s *Service) Tree() ([]*FileNode, error) {
	fingerprint := treeFingerprint(s.workspace)
	treeCacheMu.Lock()
	defer treeCacheMu.Unlock()
	if entry, ok := treeCache[s.workspace]; ok && entry.fingerprint == fingerprint {
		return entry.tree, entry.err
	}
	tree, err := s.treeUncached()
	treeCache[s.workspace] = &treeCacheEntry{fingerprint: fingerprint, tree: tree, err: err}
	return tree, err
}

// summaryFingerprint 计算影响 Summary 输出的全部文件指纹：章节、细纲、
// 根级设定文件与章节确认状态文件。
func summaryFingerprint(workspace string) string {
	h := sha256.New()
	walkHashFiles(h, workspace, filepath.Join(workspace, "chapters"))
	walkHashFiles(h, workspace, filepath.Join(workspace, "setting", "chapter-groups"))
	for _, rel := range []string{
		"book.json",
		IdeasFileName,
		CreatorFileName,
		"setting/outline.md",
		"setting/progress.md",
		"setting/" + CharacterStatesFileName,
	} {
		hashSingleFile(h, workspace, filepath.Join(workspace, filepath.FromSlash(rel)))
	}
	hashSingleFile(h, workspace, workspacepath.Path(workspace, chapterStatusFileName))
	return hex.EncodeToString(h.Sum(nil))
}

// treeFingerprint 计算整个可见工作区（跳过隐藏）的文件指纹。
func treeFingerprint(workspace string) string {
	h := sha256.New()
	walkHashFiles(h, workspace, workspace)
	return hex.EncodeToString(h.Sum(nil))
}

func walkHashFiles(h hash.Hash, workspace, root string) {
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := entry.Name()
		if name != "." && strings.HasPrefix(name, ".") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(workspace, path)
		if err != nil {
			return nil
		}
		fmt.Fprintf(h, "%s|%d|%d\n", filepath.ToSlash(rel), info.Size(), info.ModTime().UnixNano())
		return nil
	})
}

func hashSingleFile(h hash.Hash, workspace, abs string) {
	info, err := os.Stat(abs)
	if err != nil || info.IsDir() {
		return
	}
	rel, err := filepath.Rel(workspace, abs)
	if err != nil {
		return
	}
	fmt.Fprintf(h, "%s|%d|%d\n", filepath.ToSlash(rel), info.Size(), info.ModTime().UnixNano())
}
