package book

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrNoExportableContent 表示勾选的导出内容在当前工作区中均不存在或为空。
var ErrNoExportableContent = errors.New("没有可导出的内容")

// ErrTXTRequiresChaptersOnly 表示 txt 格式只能导出章节正文，不能与其他勾选内容混用。
var ErrTXTRequiresChaptersOnly = errors.New("txt 格式仅支持章节正文，其他内容请选择 zip 打包")

// 章节组细纲导出范围。
const (
	ChapterGroupsNone   = ""
	ChapterGroupsLatest = "latest"
	ChapterGroupsAll    = "all"
)

// ExportSelection 描述导出时用户勾选的内容分组。Chapters 为章节正文；ChapterGroups
// 为细纲范围（latest=仅最新一组，all=全部历史）；其余为书籍设定单文件；Meta 为 book.json。
type ExportSelection struct {
	Chapters        bool
	ChapterGroups   string
	Outline         bool
	Rules           bool
	Progress        bool
	CharacterStates bool
	Ideas           bool
	Meta            bool
}

// HasContent 返回是否有任意内容被勾选。
func (s ExportSelection) HasContent() bool {
	return s.Chapters || s.ChapterGroups != ChapterGroupsNone || s.Outline || s.Rules ||
		s.Progress || s.CharacterStates || s.Ideas || s.Meta
}

// ExportFile 是导出包内的单个文件，Path 为相对工作区路径（zip 内保持原目录结构）。
type ExportFile struct {
	Path    string
	Content []byte
}

// ExportPack 是一次导出的组装结果，Files 为导出的文件清单。
type ExportPack struct {
	Files []ExportFile
}

// BuildExportPack 按勾选内容组装导出文件清单。章节正文按原目录结构收集（跳过空章节），
// 细纲按范围收集，单文件设定在存在时收集；全部勾选内容缺失时返回 ErrNoExportableContent。
func (s *Service) BuildExportPack(sel ExportSelection) (ExportPack, error) {
	if !sel.HasContent() {
		return ExportPack{}, ErrNoExportableContent
	}
	summary, err := s.Summary()
	if err != nil {
		return ExportPack{}, err
	}

	files := []ExportFile{}
	if sel.Chapters {
		for _, chapter := range summary.Chapters {
			if chapter.Words == 0 {
				continue
			}
			content, err := s.ReadFile(chapter.Path)
			if err != nil {
				return ExportPack{}, fmt.Errorf("读取章节 %s 失败: %w", chapter.Path, err)
			}
			files = append(files, ExportFile{Path: chapter.Path, Content: []byte(content)})
		}
	}

	if sel.ChapterGroups != ChapterGroupsNone {
		groups := summary.ChapterPlans
		if sel.ChapterGroups == ChapterGroupsLatest && len(groups) > 0 {
			groups = groups[len(groups)-1:]
		}
		for _, group := range groups {
			content, err := s.ReadFile(group.Path)
			if err != nil {
				return ExportPack{}, fmt.Errorf("读取细纲 %s 失败: %w", group.Path, err)
			}
			files = append(files, ExportFile{Path: group.Path, Content: []byte(content)})
		}
	}

	singleFiles := []string{}
	if sel.Outline {
		singleFiles = append(singleFiles, filepath.ToSlash(filepath.Join("setting", "outline.md")))
	}
	if sel.Rules {
		singleFiles = append(singleFiles, CreatorFileName)
	}
	if sel.Progress {
		singleFiles = append(singleFiles, filepath.ToSlash(filepath.Join("setting", "progress.md")))
	}
	if sel.CharacterStates {
		singleFiles = append(singleFiles, filepath.ToSlash(filepath.Join("setting", CharacterStatesFileName)))
	}
	if sel.Ideas {
		singleFiles = append(singleFiles, IdeasFileName)
	}
	for _, rel := range singleFiles {
		content, err := s.ReadFile(rel)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return ExportPack{}, fmt.Errorf("读取设定 %s 失败: %w", rel, err)
		}
		files = append(files, ExportFile{Path: rel, Content: []byte(content)})
	}

	if sel.Meta {
		data, err := json.MarshalIndent(ReadBookMetaFromDir(s.workspace), "", "  ")
		if err != nil {
			return ExportPack{}, fmt.Errorf("序列化书籍元信息失败: %w", err)
		}
		files = append(files, ExportFile{Path: "book.json", Content: data})
	}

	if len(files) == 0 {
		return ExportPack{}, ErrNoExportableContent
	}
	return ExportPack{Files: files}, nil
}

// BuildExportZip 将文件清单打包为 zip 字节流，保留原相对目录结构，中文文件名按 UTF-8 写入。
func BuildExportZip(files []ExportFile) ([]byte, error) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, f := range files {
		name := strings.TrimSpace(f.Path)
		if name == "" {
			continue
		}
		hdr := &zip.FileHeader{
			Name:   filepath.ToSlash(strings.TrimPrefix(name, "/")),
			Method: zip.Deflate,
		}
		hdr.SetMode(0o644)
		entry, err := w.CreateHeader(hdr)
		if err != nil {
			return nil, fmt.Errorf("创建 zip 条目 %s 失败: %w", f.Path, err)
		}
		if _, err := entry.Write(f.Content); err != nil {
			return nil, fmt.Errorf("写入 zip 条目 %s 失败: %w", f.Path, err)
		}
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("完成 zip 打包失败: %w", err)
	}
	return buf.Bytes(), nil
}
