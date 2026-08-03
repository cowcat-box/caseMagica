package app

import (
	"context"
	"log"

	novaskills "casemagica/internal/skills"
)

// SkillsAppService exposes user and workspace skill management.
type SkillsAppService struct {
	app *App
}

func (a *App) SkillSnapshot(ctx context.Context) (novaskills.Snapshot, error) {
	return a.skills().Snapshot(ctx)
}

func (a *App) SkillDocument(ctx context.Context, scope novaskills.Scope, name string) (novaskills.Document, error) {
	return a.skills().Document(ctx, scope, name)
}

func (a *App) SkillFileDocument(ctx context.Context, scope novaskills.Scope, name, path string) (novaskills.FileDocument, error) {
	return a.skills().FileDocument(ctx, scope, name, path)
}

func (a *App) CreateSkillDocument(ctx context.Context, scope novaskills.Scope, name, description string, agents []string) (novaskills.Document, error) {
	return a.skills().Create(ctx, scope, name, description, agents)
}

func (a *App) SaveSkillFileDocument(ctx context.Context, scope novaskills.Scope, name, path, content string) (novaskills.FileDocument, error) {
	return a.skills().SaveFile(ctx, scope, name, path, content)
}

func (a *App) SaveSkillDocumentAs(ctx context.Context, scope novaskills.Scope, name string, targetScope novaskills.Scope, targetName, content string) (novaskills.Document, error) {
	return a.skills().SaveAs(ctx, scope, name, targetScope, targetName, content)
}

func (a *App) DeleteSkillDocument(ctx context.Context, scope novaskills.Scope, name string) error {
	return a.skills().Delete(ctx, scope, name)
}

func (a *App) PreviewSkillZip(ctx context.Context, scope novaskills.Scope, data []byte) (novaskills.InstallPreview, error) {
	return a.skills().PreviewZip(ctx, scope, data)
}

func (a *App) InstallSkillZip(ctx context.Context, scope novaskills.Scope, data []byte, candidateIDs []string) (novaskills.InstallResult, error) {
	return a.skills().InstallZip(ctx, scope, data, candidateIDs)
}

func (a *App) PreviewSkillRemoteArchive(ctx context.Context, scope novaskills.Scope, source novaskills.RemoteArchiveSource) (novaskills.InstallPreview, error) {
	return a.skills().PreviewRemoteArchive(ctx, scope, source)
}

func (a *App) InstallSkillRemoteArchive(ctx context.Context, scope novaskills.Scope, source novaskills.RemoteArchiveSource, candidateIDs []string) (novaskills.InstallResult, error) {
	return a.skills().InstallRemoteArchive(ctx, scope, source, candidateIDs)
}

func (a *App) PreviewSkillGitHub(ctx context.Context, scope novaskills.Scope, source novaskills.GitHubSource) (novaskills.InstallPreview, error) {
	return a.skills().PreviewGitHub(ctx, scope, source)
}

func (a *App) InstallSkillGitHub(ctx context.Context, scope novaskills.Scope, source novaskills.GitHubSource, candidateIDs []string) (novaskills.InstallResult, error) {
	return a.skills().InstallGitHub(ctx, scope, source, candidateIDs)
}

func (s *SkillsAppService) Snapshot(ctx context.Context) (novaskills.Snapshot, error) {
	return novaskills.SnapshotFor(ctx, s.directories())
}

func (s *SkillsAppService) Document(ctx context.Context, scope novaskills.Scope, name string) (novaskills.Document, error) {
	return novaskills.ReadDocument(ctx, s.directories(), scope, name)
}

func (s *SkillsAppService) FileDocument(ctx context.Context, scope novaskills.Scope, name, path string) (novaskills.FileDocument, error) {
	return novaskills.ReadSkillFile(ctx, s.directories(), scope, name, path)
}

func (s *SkillsAppService) Create(ctx context.Context, scope novaskills.Scope, name, description string, agents []string) (novaskills.Document, error) {
	doc, err := novaskills.CreateDocument(ctx, s.directories(), scope, name, description, agents...)
	if err != nil {
		return novaskills.Document{}, err
	}
	log.Printf("[skills] Skill created scope=%s name=%s path=%s", scope, name, doc.Path)
	return doc, nil
}

func (s *SkillsAppService) SaveFile(ctx context.Context, scope novaskills.Scope, name, path, content string) (novaskills.FileDocument, error) {
	doc, err := novaskills.SaveSkillFile(ctx, s.directories(), scope, name, path, content)
	if err != nil {
		return novaskills.FileDocument{}, err
	}
	log.Printf("[skills] Skill file saved scope=%s name=%s file=%s", scope, name, path)
	return doc, nil
}

func (s *SkillsAppService) SaveAs(ctx context.Context, scope novaskills.Scope, name string, targetScope novaskills.Scope, targetName, content string) (novaskills.Document, error) {
	doc, err := novaskills.SaveDocumentAs(ctx, s.directories(), scope, name, targetScope, targetName, content)
	if err != nil {
		return novaskills.Document{}, err
	}
	log.Printf("[skills] Skill saved as source_scope=%s source_name=%s target_scope=%s target_name=%s path=%s", scope, name, targetScope, targetName, doc.Path)
	return doc, nil
}

func (s *SkillsAppService) Delete(ctx context.Context, scope novaskills.Scope, name string) error {
	if err := novaskills.DeleteDocument(ctx, s.directories(), scope, name); err != nil {
		return err
	}
	log.Printf("[skills] Skill deleted scope=%s name=%s", scope, name)
	return nil
}

func (s *SkillsAppService) PreviewZip(ctx context.Context, scope novaskills.Scope, data []byte) (novaskills.InstallPreview, error) {
	return novaskills.PreviewZip(ctx, s.directories(), scope, data)
}

func (s *SkillsAppService) InstallZip(ctx context.Context, scope novaskills.Scope, data []byte, candidateIDs []string) (novaskills.InstallResult, error) {
	result, err := novaskills.InstallZip(ctx, s.directories(), scope, data, candidateIDs)
	if err != nil {
		return novaskills.InstallResult{}, err
	}
	log.Printf("[skills] Skills installed from zip scope=%s count=%d", scope, len(result.Installed))
	return result, nil
}

func (s *SkillsAppService) PreviewRemoteArchive(ctx context.Context, scope novaskills.Scope, source novaskills.RemoteArchiveSource) (novaskills.InstallPreview, error) {
	return novaskills.PreviewRemoteArchive(ctx, s.directories(), scope, source)
}

func (s *SkillsAppService) InstallRemoteArchive(ctx context.Context, scope novaskills.Scope, source novaskills.RemoteArchiveSource, candidateIDs []string) (novaskills.InstallResult, error) {
	result, err := novaskills.InstallRemoteArchive(ctx, s.directories(), scope, source, candidateIDs)
	if err != nil {
		return novaskills.InstallResult{}, err
	}
	log.Printf("[skills] Skills installed from remote archive scope=%s count=%d", scope, len(result.Installed))
	return result, nil
}

func (s *SkillsAppService) PreviewGitHub(ctx context.Context, scope novaskills.Scope, source novaskills.GitHubSource) (novaskills.InstallPreview, error) {
	return novaskills.PreviewGitHub(ctx, s.directories(), scope, source)
}

func (s *SkillsAppService) InstallGitHub(ctx context.Context, scope novaskills.Scope, source novaskills.GitHubSource, candidateIDs []string) (novaskills.InstallResult, error) {
	result, err := novaskills.InstallGitHub(ctx, s.directories(), scope, source, candidateIDs)
	if err != nil {
		return novaskills.InstallResult{}, err
	}
	log.Printf("[skills] Skills installed from github scope=%s url=%q count=%d", scope, source.URL, len(result.Installed))
	return result, nil
}

func (s *SkillsAppService) directories() []novaskills.Directory {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.cfg == nil {
		return nil
	}
	return novaskills.NewDirectories(a.cfg.SkillsDir, a.cfg.DataDir(), a.workspace)
}

// draftStore 返回用户级草稿存储（跨工作区共享）。
func (s *SkillsAppService) draftStore() *novaskills.DraftStore {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.cfg == nil {
		return novaskills.NewDraftStore("")
	}
	return novaskills.NewDraftStore(a.cfg.DataDir())
}

// SkillDraftCreate 创建 Skill 草稿。
func (a *App) SkillDraftCreate(input novaskills.CreateDraftInput) (novaskills.DraftMeta, error) {
	return a.skills().draftStore().CreateDraft(input)
}

// SkillDraftList 列出全部 Skill 草稿。
func (a *App) SkillDraftList() ([]novaskills.DraftMeta, error) {
	return a.skills().draftStore().ListDrafts()
}

// SkillDraftRead 读取 Skill 草稿完整内容。
func (a *App) SkillDraftRead(name string) (novaskills.DraftDocument, error) {
	return a.skills().draftStore().ReadDraft(name)
}

// SkillDraftUpdate 更新 Skill 草稿正文。
func (a *App) SkillDraftUpdate(name, description, agent, context, model, body string) (novaskills.DraftMeta, error) {
	return a.skills().draftStore().UpdateDraftBody(name, description, agent, context, model, body)
}

// SkillDraftDiscard 丢弃 Skill 草稿（移至备份区）。
func (a *App) SkillDraftDiscard(name string) (string, error) {
	return a.skills().draftStore().DiscardDraft(name)
}

// SkillDraftConfirm 确认导入 Skill 草稿为正式 Skill。
func (a *App) SkillDraftConfirm(ctx context.Context, name string, scope novaskills.Scope) (novaskills.Document, error) {
	return a.skills().draftStore().ConfirmDraft(name, scope, a.skills().directories())
}
