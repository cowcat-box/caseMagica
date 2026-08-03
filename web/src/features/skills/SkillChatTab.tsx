import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Eye, FileText, RefreshCw, Trash2, Upload, Wand2 } from 'lucide-react'
import { ConfigManagerChat } from '@/components/Chat/ConfigManagerChat'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { MarkdownRenderer } from '@/components/common/MarkdownRenderer'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { Input } from '@/components/ui/input'
import { confirmSkillDraft, discardSkillDraft, listSkillDrafts, readSkillDraft, updateSkillDraft, type SkillDraftMeta } from '@/lib/api-client/skills'
import type { SkillScope } from '@/lib/api'
import type { AgentMessageView } from '@/lib/agent-message-view'

interface SkillChatTabProps {
  workspace: string
  scopes: { scope: SkillScope; writable: boolean }[]
  defaultScope: SkillScope
  onMutated: () => void
}

interface DraftConfirmRequest {
  draft: SkillDraftMeta
}

export function SkillChatTab({ workspace, scopes, defaultScope, onMutated }: SkillChatTabProps) {
  const { t } = useTranslation()
  const [drafts, setDrafts] = useState<SkillDraftMeta[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [previewName, setPreviewName] = useState<string | null>(null)
  const [previewContent, setPreviewContent] = useState('')
  const [editingDraft, setEditingDraft] = useState<SkillDraftMeta | null>(null)
  const [editDescription, setEditDescription] = useState('')
  const [editBody, setEditBody] = useState('')
  const [editSaving, setEditSaving] = useState(false)
  const [confirmRequest, setConfirmRequest] = useState<DraftConfirmRequest | null>(null)
  const [confirmScope, setConfirmScope] = useState<SkillScope>(defaultScope)
  const [discardRequest, setDiscardRequest] = useState<DraftConfirmRequest | null>(null)
  const notifiedDraftsRef = useRef(new Set<string>())
  const writableScopes = useMemo(() => scopes.filter((item) => item.writable), [scopes])

  const loadDrafts = useCallback(async (notifyNew = false) => {
    setLoading(true)
    setError(null)
    try {
      const items = await listSkillDrafts()
      if (notifyNew) {
        for (const draft of items) {
          if (!notifiedDraftsRef.current.has(draft.name)) {
            notifiedDraftsRef.current.add(draft.name)
            window.dispatchEvent(new CustomEvent('nova:skill-draft', { detail: draft }))
          }
        }
      }
      setDrafts(items)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { void loadDrafts() }, [loadDrafts])

  /** 监听 create_skill_draft 工具成功结果，刷新草稿列表。 */
  const handleToolSuccess = useCallback((view: AgentMessageView) => {
    if (view.kind !== 'tool' || view.status !== 'success' || view.toolName !== 'create_skill_draft') return
    void loadDrafts(true)
  }, [loadDrafts])

  const handleMutated = useCallback(() => {
    onMutated()
  }, [onMutated])

  const openPreview = async (draft: SkillDraftMeta) => {
    setError(null)
    try {
      const doc = await readSkillDraft(draft.name)
      setPreviewContent(doc.content)
      setPreviewName(draft.name)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }

  const openEdit = async (draft: SkillDraftMeta) => {
    setError(null)
    try {
      const doc = await readSkillDraft(draft.name)
      setEditingDraft(draft)
      setEditDescription(doc.meta.description)
      setEditBody(doc.body)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }

  const saveEdit = async () => {
    if (!editingDraft) return
    setEditSaving(true)
    setError(null)
    try {
      await updateSkillDraft(editingDraft.name, {
        description: editDescription,
        body: editBody,
      })
      setEditingDraft(null)
      await loadDrafts()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setEditSaving(false)
    }
  }

  const performConfirm = async () => {
    if (!confirmRequest) return
    setError(null)
    try {
      await confirmSkillDraft(confirmRequest.draft.name, confirmScope)
      setConfirmRequest(null)
      await loadDrafts()
      onMutated()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
      throw err
    }
  }

  const performDiscard = async () => {
    if (!discardRequest) return
    setError(null)
    try {
      await discardSkillDraft(discardRequest.draft.name)
      setDiscardRequest(null)
      await loadDrafts()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
      throw err
    }
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex min-h-0 flex-1 flex-col gap-3 p-3 md:flex-row">
        <div className="flex min-h-0 flex-1 flex-col rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)]">
          <div className="flex items-center justify-between gap-2 border-b border-[var(--nova-border)] px-3 py-2">
            <div className="flex items-center gap-1.5 text-xs font-medium text-[var(--nova-text)]">
              <Wand2 className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
              {t('skills.chat.title')}
            </div>
            <Button type="button" variant="ghost" size="xs" className="h-6 gap-1 px-1.5 text-[10px] text-[var(--nova-text-faint)]" onClick={() => void loadDrafts()} disabled={loading}>
              <RefreshCw className={`h-3 w-3 ${loading ? 'animate-spin' : ''}`} />
              {t('common.refresh')}
            </Button>
          </div>
          <div className="min-h-0 flex-1">
            <ConfigManagerChat
              workspace={workspace}
              origin="skills_chat"
              onMutated={handleMutated}
              onToolSuccess={handleToolSuccess}
              className="h-full"
            />
          </div>
        </div>

        <div className="flex min-h-0 w-full flex-col rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] md:w-80">
          <div className="border-b border-[var(--nova-border)] px-3 py-2 text-xs font-medium text-[var(--nova-text)]">
            {t('skills.chat.drafts')}
            {drafts.length > 0 && <span className="ml-1.5 text-[10px] text-[var(--nova-text-faint)]">{t('skills.chat.draftCount', { count: drafts.length })}</span>}
          </div>
          <div className="min-h-0 flex-1 space-y-1.5 overflow-y-auto p-2">
            {error && <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2.5 py-1.5 text-[11px] text-[var(--nova-danger-text)]">{error}</div>}
            {drafts.length === 0 && !loading && (
              <div className="py-6 text-center text-[11px] text-[var(--nova-text-faint)]">
                {t('skills.chat.draftsEmpty')}
              </div>
            )}
            {drafts.map((draft) => (
              <div key={draft.name} className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2">
                <div className="flex items-center gap-1.5">
                  <FileText className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
                  <span className="min-w-0 flex-1 truncate font-mono text-[11px] font-medium text-[var(--nova-text)]">/{draft.name}</span>
                </div>
                <div className="mt-1 line-clamp-2 text-[10px] leading-4 text-[var(--nova-text-muted)]">{draft.description || '-'}</div>
                <div className="mt-1 text-[10px] text-[var(--nova-text-faint)]">
                  {t('skills.chat.draftFiles', { count: (draft.files || []).length + 1 })}
                </div>
                <div className="mt-1.5 flex flex-wrap items-center gap-1">
                  <Button type="button" variant="ghost" size="xs" className="h-6 gap-1 px-1.5 text-[10px] text-[var(--nova-text-muted)]" onClick={() => void openPreview(draft)}>
                    <Eye className="h-3 w-3" />
                    {t('skills.chat.preview')}
                  </Button>
                  <Button type="button" variant="ghost" size="xs" className="h-6 gap-1 px-1.5 text-[10px] text-[var(--nova-text-muted)]" onClick={() => void openEdit(draft)}>
                    <FileText className="h-3 w-3" />
                    {t('common.edit')}
                  </Button>
                  <Button type="button" variant="ghost" size="xs" className="h-6 gap-1 px-1.5 text-[10px] text-[var(--nova-text)]" onClick={() => { setConfirmScope(defaultScope); setConfirmRequest({ draft }) }}>
                    <Upload className="h-3 w-3" />
                    {t('skills.chat.import')}
                  </Button>
                  <Button type="button" variant="ghost" size="xs" className="h-6 gap-1 px-1.5 text-[10px] text-[var(--nova-danger-text)]" onClick={() => setDiscardRequest({ draft })}>
                    <Trash2 className="h-3 w-3" />
                    {t('skills.chat.discard')}
                  </Button>
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>

      <Dialog open={Boolean(previewName)} onOpenChange={(open) => { if (!open) setPreviewName(null) }}>
        <DialogContent
          className="nova-panel w-[min(680px,calc(100vw-2rem))] max-w-[min(680px,calc(100vw-2rem))] rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0 text-[var(--nova-text)] shadow-[var(--nova-shadow)]"
          aria-describedby="skill-draft-preview-desc"
        >
          <div className="border-b border-[var(--nova-border)] px-4 py-3">
            <DialogTitle className="text-sm font-semibold text-[var(--nova-text)]">{previewName ? `/${previewName}` : ''}</DialogTitle>
            <DialogDescription id="skill-draft-preview-desc" className="mt-1 text-xs text-[var(--nova-text-faint)]">
              {t('skills.chat.previewDescription')}
            </DialogDescription>
          </div>
          <div className="max-h-[70vh] overflow-y-auto px-4 py-4 text-xs">
            <MarkdownRenderer content={previewContent} />
          </div>
        </DialogContent>
      </Dialog>

      <Dialog open={Boolean(editingDraft)} onOpenChange={(open) => { if (!open && !editSaving) setEditingDraft(null) }}>
        <DialogContent
          className="nova-panel w-[min(680px,calc(100vw-2rem))] max-w-[min(680px,calc(100vw-2rem))] rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0 text-[var(--nova-text)] shadow-[var(--nova-shadow)]"
          aria-describedby="skill-draft-edit-desc"
        >
          <div className="border-b border-[var(--nova-border)] px-4 py-3">
            <DialogTitle className="text-sm font-semibold text-[var(--nova-text)]">{editingDraft ? `/${editingDraft.name}` : ''}</DialogTitle>
            <DialogDescription id="skill-draft-edit-desc" className="mt-1 text-xs text-[var(--nova-text-faint)]">
              {t('skills.chat.editDescription')}
            </DialogDescription>
          </div>
          <div className="space-y-3 px-4 py-4 text-xs">
            <div className="space-y-1">
              <div className="text-[11px] font-medium text-[var(--nova-text-faint)]">{t('skills.chat.description')}</div>
              <Input value={editDescription} onChange={(event) => setEditDescription(event.target.value)} className="h-8 text-xs" />
            </div>
            <div className="space-y-1">
              <div className="text-[11px] font-medium text-[var(--nova-text-faint)]">{t('skills.chat.body')}</div>
              <Textarea value={editBody} onChange={(event) => setEditBody(event.target.value)} rows={14} className="text-xs" />
            </div>
          </div>
          <div className="flex items-center justify-end gap-2 border-t border-[var(--nova-border)] px-4 py-3">
            <Button type="button" size="xs" variant="ghost" className="nova-nav-item text-[var(--nova-text-faint)]" onClick={() => setEditingDraft(null)} disabled={editSaving}>
              {t('common.cancel')}
            </Button>
            <Button type="button" size="xs" className="nova-nav-item" onClick={() => void saveEdit()} disabled={editSaving}>
              {editSaving ? t('skills.chat.saving') : t('common.save')}
            </Button>
          </div>
        </DialogContent>
      </Dialog>

      {confirmRequest && (
        <ConfirmDialog
          open={true}
          onOpenChange={(open) => { if (!open) setConfirmRequest(null) }}
          title={t('skills.chat.confirmTitle')}
          description={t('skills.chat.confirmDescription', { name: confirmRequest.draft.name })}
          confirmLabel={t('skills.chat.import')}
          tone="default"
          detailContent={
            <div className="space-y-1.5">
              <div className="text-[11px] font-medium text-[var(--nova-text-faint)]">{t('skills.scope.label')}</div>
              <Select value={confirmScope} onValueChange={(value) => setConfirmScope(value as SkillScope)}>
                <SelectTrigger size="sm" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {writableScopes.map((item) => (
                    <SelectItem key={item.scope} value={item.scope} className="text-xs">{t(`skills.scope.${item.scope}`)}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          }
          onConfirm={performConfirm}
        />
      )}

      {discardRequest && (
        <ConfirmDialog
          open={true}
          onOpenChange={(open) => { if (!open) setDiscardRequest(null) }}
          title={t('skills.chat.discardTitle')}
          description={t('skills.chat.discardDescription', { name: discardRequest.draft.name })}
          confirmLabel={t('skills.chat.discard')}
          tone="danger"
          onConfirm={performDiscard}
        />
      )}
    </div>
  )
}
