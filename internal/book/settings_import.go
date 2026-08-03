package book

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"casemagica/internal/workspacepath"
)

// SettingsKind 表示书籍设定文件的类型，用于材料导入的识别与落位。
type SettingsKind string

const (
	SettingsKindUnknown         SettingsKind = ""
	SettingsKindOutline         SettingsKind = "outline"
	SettingsKindRules           SettingsKind = "rules"
	SettingsKindProgress        SettingsKind = "progress"
	SettingsKindCharacterStates SettingsKind = "character_states"
	SettingsKindIdeas           SettingsKind = "ideas"
	SettingsKindChapterGroup    SettingsKind = "chapter_group"
)

// SettingsKinds 列出全部可导入的设定类型（用于前端手动选择兜底）。
var SettingsKinds = []SettingsKind{
	SettingsKindOutline,
	SettingsKindRules,
	SettingsKindProgress,
	SettingsKindCharacterStates,
	SettingsKindIdeas,
	SettingsKindChapterGroup,
}

// SettingsImportResult 描述一次材料导入的落位与备份信息。
type SettingsImportResult struct {
	TargetPath string `json:"target_path"`
	BackupPath string `json:"backup_path,omitempty"`
}

// SettingsImportPreview 描述材料识别预览（导入确认弹窗展示）。
type SettingsImportPreview struct {
	Kind       SettingsKind `json:"kind"`
	TargetPath string       `json:"target_path,omitempty"`
}

// ErrSettingsImportTypeRequired 表示无法自动识别材料类型，需要调用方提供类型。
var ErrSettingsImportTypeRequired = errors.New("无法识别材料类型，请手动选择导入目标")

// ErrSettingsImportUnsupportedKind 表示给定的导入类型不支持。
var ErrSettingsImportUnsupportedKind = errors.New("不支持的导入类型")

var chapterGroupFilePattern = regexp.MustCompile(`(?i)^group(\d+)`)
var groupNameHeadingPattern = regexp.MustCompile(`(?i)细纲|章节组|分卷规划|卷计划`)

// IdentifySettingsKind 根据文件名与内容启发式识别设定文件类型；
// 无法识别时返回 SettingsKindUnknown。
func IdentifySettingsKind(filename, content string) SettingsKind {
	base := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(filepath.Base(filename)), filepath.Ext(filename)))
	switch base {
	case "outline":
		return SettingsKindOutline
	case "creator", "rules", "rule", "创作规则", "规则":
		return SettingsKindRules
	case "progress", "进度":
		return SettingsKindProgress
	case "character-states", "character_states", "characterstate", "角色状态", "角色状态表":
		return SettingsKindCharacterStates
	case "ideas", "idea", "brainstorm", "灵感", "构思":
		return SettingsKindIdeas
	}
	if chapterGroupFilePattern.MatchString(base) || groupNameHeadingPattern.MatchString(base) {
		return SettingsKindChapterGroup
	}

	for _, line := range strings.Split(content, "\n") {
		text := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "#"))
		if text == "" {
			continue
		}
		switch {
		case strings.Contains(text, "大纲"):
			return SettingsKindOutline
		case strings.Contains(text, "创作规则") || strings.Contains(text, "写作规范"):
			return SettingsKindRules
		case strings.Contains(text, "进度") && strings.Contains(text, "章"):
			return SettingsKindProgress
		case strings.Contains(text, "角色状态"):
			return SettingsKindCharacterStates
		case strings.Contains(text, "灵感") || strings.Contains(text, "构思"):
			return SettingsKindIdeas
		case strings.Contains(text, "细纲") || strings.Contains(text, "章节组"):
			return SettingsKindChapterGroup
		}
	}
	return SettingsKindUnknown
}

// SettingsExportPath 返回设定类型对应的相对路径；细纲返回目录前缀。
func (s *Service) SettingsExportPath(kind SettingsKind) (string, error) {
	switch kind {
	case SettingsKindOutline:
		return "setting/outline.md", nil
	case SettingsKindRules:
		return CreatorFileName, nil
	case SettingsKindProgress:
		return "setting/progress.md", nil
	case SettingsKindCharacterStates:
		return "setting/" + CharacterStatesFileName, nil
	case SettingsKindIdeas:
		return IdeasFileName, nil
	case SettingsKindChapterGroup:
		return "setting/chapter-groups/", nil
	default:
		return "", ErrSettingsImportUnsupportedKind
	}
}

// IdentifySettingsImport 识别材料类型并预览目标落位（不写入任何内容）。
// 提供 presetKind 时跳过启发式识别，直接按指定类型计算目标落位。
func (s *Service) IdentifySettingsImport(filename, content string, presetKind SettingsKind) (SettingsImportPreview, error) {
	kind := presetKind
	if kind == SettingsKindUnknown {
		kind = IdentifySettingsKind(filename, content)
	}
	if kind == SettingsKindUnknown {
		return SettingsImportPreview{Kind: kind}, nil
	}
	target, err := s.settingsTargetPath(kind)
	if err != nil {
		return SettingsImportPreview{}, err
	}
	return SettingsImportPreview{Kind: kind, TargetPath: target}, nil
}

// IsSettingsExportFile 校验相对路径是否属于可导出的设定文件白名单。
func IsSettingsExportFile(rel string) bool {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	switch rel {
	case "setting/outline.md", CreatorFileName, "setting/progress.md",
		"setting/" + CharacterStatesFileName, IdeasFileName:
		return true
	}
	if strings.HasPrefix(rel, "setting/chapter-groups/") {
		ext := strings.ToLower(filepath.Ext(rel))
		return ext == ".md" || ext == ".txt"
	}
	return false
}

// SettingsExportSingle 读取单个设定文件内容供下载（白名单校验）。
func (s *Service) SettingsExportSingle(rel string) (string, []byte, error) {
	if !IsSettingsExportFile(rel) {
		return "", nil, fmt.Errorf("不允许导出该文件: %s", rel)
	}
	content, err := s.ReadFile(rel)
	if err != nil {
		return "", nil, err
	}
	return filepath.Base(rel), []byte(content), nil
}

// SettingsExportGroups 打包全部章节组细纲（保持原目录结构）。
func (s *Service) SettingsExportGroups() ([]byte, error) {
	pack, err := s.BuildExportPack(ExportSelection{ChapterGroups: ChapterGroupsAll})
	if err != nil {
		return nil, err
	}
	return BuildExportZip(pack.Files)
}

// ImportSettingsFile 将材料写入工作区对应设定文件。写入前自动备份已存在的目标文件到
// .casemagica/backups/settings-import/<timestamp>/；细纲自动编号不覆盖已有文件。
func (s *Service) ImportSettingsFile(content []byte, kind SettingsKind) (SettingsImportResult, error) {
	rel, err := s.settingsTargetPath(kind)
	if err != nil {
		return SettingsImportResult{}, err
	}

	absTarget := filepath.Join(s.workspace, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(absTarget), 0o755); err != nil {
		return SettingsImportResult{}, fmt.Errorf("创建目标目录失败: %w", err)
	}

	result := SettingsImportResult{TargetPath: rel}
	if _, statErr := os.Stat(absTarget); statErr == nil {
		backupPath, backupErr := backupSettingsImport(s.workspace, rel)
		if backupErr != nil {
			return SettingsImportResult{}, fmt.Errorf("备份原文件失败: %w", backupErr)
		}
		result.BackupPath = backupPath
	} else if !os.IsNotExist(statErr) {
		return SettingsImportResult{}, fmt.Errorf("检查目标文件失败: %w", statErr)
	}

	if err := atomicWriteFile(absTarget, content); err != nil {
		return SettingsImportResult{}, fmt.Errorf("写入 %s 失败: %w", rel, err)
	}
	return result, nil
}

func (s *Service) settingsTargetPath(kind SettingsKind) (string, error) {
	switch kind {
	case SettingsKindOutline:
		return "setting/outline.md", nil
	case SettingsKindRules:
		return CreatorFileName, nil
	case SettingsKindProgress:
		return "setting/progress.md", nil
	case SettingsKindCharacterStates:
		return "setting/" + CharacterStatesFileName, nil
	case SettingsKindIdeas:
		return IdeasFileName, nil
	case SettingsKindChapterGroup:
		return s.nextChapterGroupPath()
	case SettingsKindUnknown:
		return "", ErrSettingsImportTypeRequired
	default:
		return "", ErrSettingsImportUnsupportedKind
	}
}

func (s *Service) nextChapterGroupPath() (string, error) {
	dir := filepath.Join(s.workspace, "setting", "chapter-groups")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "setting/chapter-groups/group1.md", nil
		}
		return "", err
	}
	maxIndex := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		matches := chapterGroupFilePattern.FindStringSubmatch(entry.Name())
		if len(matches) == 0 {
			continue
		}
		index := 0
		for _, ch := range matches[1] {
			index = index*10 + int(ch-'0')
		}
		if index > maxIndex {
			maxIndex = index
		}
	}
	return fmt.Sprintf("setting/chapter-groups/group%d.md", maxIndex+1), nil
}

func backupSettingsImport(workspace, rel string) (string, error) {
	backupRoot := filepath.Join(workspacepath.Path(workspace, "backups"), "settings-import", time.Now().Format("20060102-150405"))
	if err := os.MkdirAll(backupRoot, 0o755); err != nil {
		return "", err
	}
	absSource := filepath.Join(workspace, filepath.FromSlash(rel))
	backupPath := filepath.Join(backupRoot, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(backupPath), 0o755); err != nil {
		return "", err
	}
	data, err := os.ReadFile(absSource)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(backupPath, data, 0o644); err != nil {
		return "", err
	}
	display, err := filepath.Rel(workspace, backupPath)
	if err != nil {
		return filepath.ToSlash(backupPath), nil
	}
	return filepath.ToSlash(display), nil
}

// atomicWriteFile 先写临时文件再原子重命名，避免写入中断留下半截文件。
func atomicWriteFile(absPath string, content []byte) error {
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return err
	}
	tempPath := fmt.Sprintf("%s.tmp-%s", absPath, hex.EncodeToString(random[:]))
	if err := os.WriteFile(tempPath, content, 0o644); err != nil {
		return err
	}
	return os.Rename(tempPath, absPath)
}
