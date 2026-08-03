package skills

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// DraftDirName 是用户级 Skill 草稿区目录名（与 user scope skills 同级，跨工作区共享）。
const DraftDirName = "skill-drafts"

const draftMetaFileName = "meta.json"
const draftSkillFileName = "SKILL.md"

// ErrDraftNotFound 表示草稿不存在。
var ErrDraftNotFound = errors.New("skill draft not found")

// ErrDraftExists 表示同名草稿已存在。
var ErrDraftExists = errors.New("skill draft already exists")

// DraftMeta 描述一个 Skill 草稿。
type DraftMeta struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Agent       string   `json:"agent,omitempty"`
	Context     string   `json:"context,omitempty"`
	Model       string   `json:"model,omitempty"`
	SourceScope string   `json:"source_scope,omitempty"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
	Files       []string `json:"files"`
}

// DraftDocument 是草稿的完整内容。
type DraftDocument struct {
	Meta    DraftMeta         `json:"meta"`
	Body    string            `json:"body"`              // SKILL.md 正文（不含 frontmatter）
	Content string            `json:"content"`           // 完整 SKILL.md（含 frontmatter）
	Files   map[string]string `json:"files,omitempty"`   // 附加文件 相对路径->内容
}

// DraftStore 管理用户级 Skill 草稿区（<denovaDir>/skill-drafts/<name>/）。
type DraftStore struct {
	root string
}

// NewDraftStore 创建草稿存储；denovaDir 为空时返回不可用实例。
func NewDraftStore(denovaDir string) *DraftStore {
	return &DraftStore{root: filepath.Join(strings.TrimSpace(denovaDir), DraftDirName)}
}

// Root 返回草稿区根目录。
func (s *DraftStore) Root() string {
	return s.root
}

// Available 返回草稿存储是否可用。
func (s *DraftStore) Available() bool {
	return strings.TrimSpace(s.root) != "" && strings.TrimSpace(s.root) != string(filepath.Separator)
}

// CreateDraftInput 描述创建草稿的入参。
type CreateDraftInput struct {
	Name        string
	Description string
	Agent       string
	Context     string
	Model       string
	Body        string            // SKILL.md 正文（不含 frontmatter）
	Files       map[string]string // 附加文件 相对路径->内容
	SourceScope string
}

// CreateDraft 创建草稿目录并写入 SKILL.md 与附加文件；同名草稿已存在时返回 ErrDraftExists。
func (s *DraftStore) CreateDraft(input CreateDraftInput) (DraftMeta, error) {
	if !s.Available() {
		return DraftMeta{}, fmt.Errorf("skill draft store is not configured")
	}
	name := strings.TrimSpace(input.Name)
	if err := ValidateName(name); err != nil {
		return DraftMeta{}, err
	}
	if strings.TrimSpace(input.Description) == "" {
		return DraftMeta{}, errors.New("skill description is required")
	}
	dir := s.draftDir(name)
	if _, err := os.Stat(dir); err == nil {
		return DraftMeta{}, ErrDraftExists
	} else if !os.IsNotExist(err) {
		return DraftMeta{}, fmt.Errorf("stat draft dir failed: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return DraftMeta{}, fmt.Errorf("create draft dir failed: %w", err)
	}

	now := time.Now().Format(time.RFC3339)
	files := []string{}
	for path := range input.Files {
		if path != "" {
			files = append(files, path)
		}
	}
	sort.Strings(files)
	meta := DraftMeta{
		Name:        name,
		Description: strings.TrimSpace(input.Description),
		Agent:       strings.TrimSpace(input.Agent),
		Context:     strings.TrimSpace(input.Context),
		Model:       strings.TrimSpace(input.Model),
		SourceScope: strings.TrimSpace(input.SourceScope),
		CreatedAt:   now,
		UpdatedAt:   now,
		Files:       files,
	}

	cleanup := func() {
		_ = os.RemoveAll(dir)
	}
	skillMD := marshalDraftSkillFile(meta, input.Body)
	if err := writeDraftFile(dir, draftSkillFileName, []byte(skillMD)); err != nil {
		cleanup()
		return DraftMeta{}, err
	}
	for rel, content := range input.Files {
		if !validDraftFilePath(rel) {
			cleanup()
			return DraftMeta{}, fmt.Errorf("invalid draft file path: %s", rel)
		}
		if err := writeDraftFile(dir, filepath.FromSlash(rel), []byte(content)); err != nil {
			cleanup()
			return DraftMeta{}, err
		}
	}
	if err := writeDraftMeta(dir, meta); err != nil {
		cleanup()
		return DraftMeta{}, err
	}
	return meta, nil
}

// ListDrafts 列出全部草稿摘要（按更新时间倒序）。
func (s *DraftStore) ListDrafts() ([]DraftMeta, error) {
	if !s.Available() {
		return []DraftMeta{}, nil
	}
	entries, err := os.ReadDir(s.root)
	if err != nil {
		if os.IsNotExist(err) {
			return []DraftMeta{}, nil
		}
		return nil, fmt.Errorf("read draft dir failed: %w", err)
	}
	drafts := []DraftMeta{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		meta, err := readDraftMeta(s.draftDir(entry.Name()))
		if err != nil {
			continue
		}
		if strings.TrimSpace(meta.Name) != "" {
			drafts = append(drafts, meta)
		}
	}
	sort.Slice(drafts, func(i, j int) bool {
		return drafts[i].UpdatedAt > drafts[j].UpdatedAt
	})
	return drafts, nil
}

// ReadDraft 读取草稿完整内容。
func (s *DraftStore) ReadDraft(name string) (DraftDocument, error) {
	dir := s.draftDir(name)
	meta, err := readDraftMeta(dir)
	if err != nil {
		return DraftDocument{}, err
	}
	skillData, err := os.ReadFile(filepath.Join(dir, draftSkillFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return DraftDocument{}, ErrDraftNotFound
		}
		return DraftDocument{}, fmt.Errorf("read draft SKILL.md failed: %w", err)
	}
	_, body, err := parseFrontmatter(string(skillData))
	if err != nil {
		body = string(skillData)
	}
	doc := DraftDocument{
		Meta:    meta,
		Body:    body,
		Content: string(skillData),
		Files:   map[string]string{},
	}
	for _, rel := range meta.Files {
		if !validDraftFilePath(rel) {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if readErr != nil {
			continue
		}
		doc.Files[rel] = string(data)
	}
	return doc, nil
}

// UpdateDraftBody 更新草稿的 SKILL.md 正文（frontmatter 字段与 meta 同步更新）。
func (s *DraftStore) UpdateDraftBody(name, description, agent, context, model, body string) (DraftMeta, error) {
	dir := s.draftDir(name)
	meta, err := readDraftMeta(dir)
	if err != nil {
		return DraftMeta{}, err
	}
	meta.Description = strings.TrimSpace(description)
	meta.Agent = strings.TrimSpace(agent)
	meta.Context = strings.TrimSpace(context)
	meta.Model = strings.TrimSpace(model)
	meta.UpdatedAt = time.Now().Format(time.RFC3339)
	skillMD := marshalDraftSkillFile(meta, body)
	if err := writeDraftFile(dir, draftSkillFileName, []byte(skillMD)); err != nil {
		return DraftMeta{}, err
	}
	if err := writeDraftMeta(dir, meta); err != nil {
		return DraftMeta{}, err
	}
	return meta, nil
}

// DiscardDraft 丢弃草稿：移动到 <denovaDir>/backups/skill-drafts/<name> 以便恢复。
func (s *DraftStore) DiscardDraft(name string) (string, error) {
	dir := s.draftDir(name)
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return "", ErrDraftNotFound
		}
		return "", err
	}
	backupRoot := filepath.Join(s.root, "..", "backups", DraftDirName)
	backupDir := filepath.Join(backupRoot, name)
	if err := os.MkdirAll(backupRoot, 0o755); err != nil {
		return "", fmt.Errorf("create draft backup dir failed: %w", err)
	}
	if _, err := os.Stat(backupDir); err == nil {
		backupDir = filepath.Join(backupRoot, fmt.Sprintf("%s-%d", name, time.Now().Unix()))
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.Rename(dir, backupDir); err != nil {
		return "", fmt.Errorf("discard draft failed: %w", err)
	}
	return backupDir, nil
}

// ConfirmDraft 将草稿确认导入为正式 Skill（scope: user | workspace），成功后删除草稿目录。
// 目标 scope 已存在同名 Skill 时返回错误，避免静默覆盖。
func (s *DraftStore) ConfirmDraft(name string, scope Scope, dirs []Directory) (Document, error) {
	doc, err := s.ReadDraft(name)
	if err != nil {
		return Document{}, err
	}
	dir, err := writableDirectoryForScope(dirs, scope)
	if err != nil {
		return Document{}, err
	}
	existing := filepath.Join(dir.Path, filepath.FromSlash(doc.Meta.Name))
	if _, statErr := os.Stat(existing); statErr == nil {
		return Document{}, fmt.Errorf("同名 Skill 已存在: %s（scope=%s），请改名后再导入", doc.Meta.Name, scope)
	} else if !os.IsNotExist(statErr) {
		return Document{}, fmt.Errorf("check existing skill failed: %w", statErr)
	}
	created, err := SaveDocument(context.Background(), dirs, scope, doc.Meta.Name, doc.Content)
	if err != nil {
		return Document{}, err
	}
	for rel, content := range doc.Files {
		if !validDraftFilePath(rel) {
			continue
		}
		if err := saveDraftExtraFile(dirs, scope, doc.Meta.Name, rel, content); err != nil {
			return Document{}, fmt.Errorf("save draft extra file %s failed: %w", rel, err)
		}
	}
	if err := os.RemoveAll(s.draftDir(name)); err != nil {
		return Document{}, fmt.Errorf("remove draft dir failed: %w", err)
	}
	return created, nil
}

func saveDraftExtraFile(dirs []Directory, scope Scope, name, rel, content string) error {
	dir, err := writableDirectoryForScope(dirs, scope)
	if err != nil {
		return err
	}
	rel = filepath.ToSlash(rel)
	if !validDraftFilePath(rel) {
		return fmt.Errorf("invalid skill file path: %s", rel)
	}
	abs := filepath.Join(dir.Path, name, filepath.FromSlash(rel))
	parent := filepath.Dir(abs)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	return os.WriteFile(abs, []byte(content), 0o644)
}

func (s *DraftStore) draftDir(name string) string {
	return filepath.Join(s.root, filepath.FromSlash(name))
}

func writeDraftMeta(dir string, meta DraftMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal draft meta failed: %w", err)
	}
	return writeDraftFile(dir, draftMetaFileName, data)
}

func readDraftMeta(dir string) (DraftMeta, error) {
	data, err := os.ReadFile(filepath.Join(dir, draftMetaFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return DraftMeta{}, ErrDraftNotFound
		}
		return DraftMeta{}, fmt.Errorf("read draft meta failed: %w", err)
	}
	var meta DraftMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return DraftMeta{}, fmt.Errorf("parse draft meta failed: %w", err)
	}
	return meta, nil
}

func writeDraftFile(dir, rel string, data []byte) error {
	abs := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return fmt.Errorf("create draft file dir failed: %w", err)
	}
	tmp := fmt.Sprintf("%s.tmp-%d", abs, time.Now().UnixNano())
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write draft file failed: %w", err)
	}
	if err := os.Rename(tmp, abs); err != nil {
		return fmt.Errorf("commit draft file failed: %w", err)
	}
	return nil
}

func validDraftFilePath(rel string) bool {
	rel = strings.TrimSpace(filepath.ToSlash(rel))
	if rel == "" || rel == draftSkillFileName || strings.HasPrefix(rel, "/") {
		return false
	}
	parts := strings.Split(rel, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

// marshalDraftSkillFile 组装完整 SKILL.md（frontmatter + 正文）。
func marshalDraftSkillFile(meta DraftMeta, body string) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "name: %q\n", meta.Name)
	fmt.Fprintf(&b, "description: %q\n", meta.Description)
	if meta.Agent != "" {
		fmt.Fprintf(&b, "agent: %q\n", meta.Agent)
	}
	if meta.Context != "" {
		fmt.Fprintf(&b, "context: %q\n", meta.Context)
	}
	if meta.Model != "" {
		fmt.Fprintf(&b, "model: %q\n", meta.Model)
	}
	b.WriteString("---\n")
	body = strings.TrimSpace(body)
	if body != "" {
		b.WriteString("\n" + body + "\n")
	}
	return b.String()
}
