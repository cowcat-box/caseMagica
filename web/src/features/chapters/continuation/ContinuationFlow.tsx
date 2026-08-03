import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckCircle2, ChevronDown, Loader2, Play, RefreshCw, RotateCcw, Send, Sparkles, Trash2, XCircle } from 'lucide-react'
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Label } from '@/components/ui/label'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import type { ChapterSummary } from '@/lib/api'
import { getStyleReferences } from '@/features/interactive/api'
import {
  abortContinuationTask,
  commitContinuation,
  deleteContinuationTask,
  exploreContinuation,
  listContinuationTasks,
  replayContinuationTask,
  type ContinuationCandidate,
  type ContinuationDepth,
  type ContinuationEvent,
  type ContinuationMode,
  type ContinuationTaskMeta,
  type ContinuationTarget,
} from '@/lib/api-client/continuation'
import type { SSEEvent } from '@/lib/api-client/types'

const CONFIG_STORAGE_KEY = 'nova.continuation.config'

interface ContinuationConfig {
  anchor: string
  depth: ContinuationDepth
  mode: ContinuationMode
  styleRefNames: string[]
  customStyle: string
  preference: string
}

const DEFAULT_CONFIG: ContinuationConfig = {
  anchor: '',
  depth: 'medium',
  mode: 'excerpt',
  styleRefNames: [],
  customStyle: '',
  preference: '',
}

interface ContinuationFlowProps {
  open: boolean
  workspace: string
  chapters: ChapterSummary[]
  defaultAnchor?: string
  onOpenChange: (open: boolean) => void
  onWorkspaceChanged?: (paths: string[]) => void
}

interface CandidateEdit {
  title: string
  direction: string
  excerpt: string
}

interface CommitRequestState {
  taskId: string
  index: number
  title: string
  content: string
}

export function ContinuationFlow({ open, workspace, chapters, defaultAnchor, onOpenChange, onWorkspaceChanged }: ContinuationFlowProps) {
  const { t } = useTranslation()
  const [config, setConfig] = useState<ContinuationConfig>(() => readStoredConfig(defaultAnchor || ''))
  const [styles, setStyles] = useState<{ name: string }[]>([])
  const [view, setView] = useState<'config' | 'panel'>('config')
  const [taskId, setTaskId] = useState('')
  const [taskStatus, setTaskStatus] = useState('')
  const [candidates, setCandidates] = useState<ContinuationCandidate[]>([])
  const [edits, setEdits] = useState<Record<number, CandidateEdit>>({})
  const [running, setRunning] = useState(false)
  const [error, setError] = useState('')
  const [history, setHistory] = useState<ContinuationTaskMeta[]>([])
  const [commitRequest, setCommitRequest] = useState<CommitRequestState | null>(null)
  const [commitTarget, setCommitTarget] = useState<ContinuationTarget>('new_chapter')
  const [commitDraftMode, setCommitDraftMode] = useState<'summary' | 'empty'>('summary')
  const [committing, setCommitting] = useState(false)
  const [deleteRequest, setDeleteRequest] = useState<string | null>(null)
  const readerRef = useRef<ReadableStreamDefaultReader<SSEEvent> | null>(null)

  const chapterOptions = useMemo(() => {
    const seen = new Map<string, string>()
    for (const chapter of chapters) {
      if (!seen.has(chapter.path)) seen.set(chapter.path, chapter.display_title)
    }
    return [...seen.entries()]
  }, [chapters])

  useEffect(() => {
    if (!open) return
    void getStyleReferences().then(setStyles).catch(() => setStyles([]))
    setView('config')
    setTaskId('')
    setTaskStatus('')
    setCandidates([])
    setEdits({})
    setError('')
    setRunning(false)
    void listContinuationTasks(workspace).then(setHistory).catch(() => setHistory([]))
    setConfig((current) => ({ ...current, anchor: current.anchor || defaultAnchor || '' }))
  }, [open, workspace, defaultAnchor])

  useEffect(() => {
    if (!open) return
    window.localStorage.setItem(CONFIG_STORAGE_KEY, JSON.stringify(config))
  }, [config, open])

  useEffect(() => () => { readerRef.current?.cancel().catch(() => {}) }, [])

  const updateConfig = <K extends keyof ContinuationConfig>(key: K, value: ContinuationConfig[K]) => {
    setConfig((current) => {
      const next = { ...current, [key]: value }
      if (key === 'depth' && value === 'short') next.mode = 'direction'
      return next
    })
  }

  const handleEvent = useCallback((event: ContinuationEvent) => {
    switch (event.event) {
      case 'explore_start':
        setTaskId(event.task_id)
        setTaskStatus('running')
        setCandidates([])
        setEdits({})
        break
      case 'candidate_start':
        setCandidates((current) => {
          if (current.some((item) => item.index === event.index)) return current
          return [...current, { index: event.index, status: 'generating' }]
        })
        break
      case 'candidate_done':
        setCandidates((current) => {
          const existing = current.some((item) => item.index === event.candidate.index)
          const next = existing
            ? current.map((item) => item.index === event.candidate.index ? event.candidate : item)
            : [...current, event.candidate]
          return [...next].sort((left, right) => left.index - right.index)
        })
        setEdits((current) => ({
          ...current,
          [event.candidate.index]: {
            title: event.candidate.title || '',
            direction: event.candidate.direction || '',
            excerpt: event.candidate.excerpt || '',
          },
        }))
        break
      case 'candidate_failed':
        setCandidates((current) => {
          const existing = current.some((item) => item.index === event.index)
          const next = existing
            ? current.map((item) => item.index === event.index ? { ...item, status: 'failed' as const, error: event.error } : item)
            : [...current, { index: event.index, status: 'failed' as const, error: event.error }]
          return [...next].sort((left, right) => left.index - right.index)
        })
        break
      case 'explore_done':
        setTaskStatus(event.status)
        setRunning(false)
        void listContinuationTasks(workspace).then(setHistory).catch(() => {})
        break
    }
  }, [workspace])

  const consumeStream = useCallback(async (stream: ReadableStream<SSEEvent>) => {
    const reader = stream.getReader()
    readerRef.current = reader
    while (true) {
      const { value, done } = await reader.read()
      if (done) break
      if (!value) continue
      let payload: ContinuationEvent
      try {
        payload = JSON.parse(value.data) as ContinuationEvent
      } catch {
        continue
      }
      handleEvent(payload)
    }
  }, [handleEvent])

  const startExplore = async () => {
    if (running) return
    setError('')
    setRunning(true)
    setView('panel')
    setTaskStatus('running')
    setCandidates([])
    setEdits({})
    try {
      const stream = await exploreContinuation({
        workspace,
        anchor_chapter_path: config.anchor || undefined,
        depth: config.depth,
        mode: config.mode,
        style_ref_names: config.styleRefNames.length > 0 ? config.styleRefNames : undefined,
        custom_style: config.customStyle.trim() || undefined,
        preference: config.preference.trim() || undefined,
      })
      await consumeStream(stream)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
      setRunning(false)
    }
  }

  const replayTask = async (id: string) => {
    setError('')
    setTaskId(id)
    setRunning(false)
    setView('panel')
    setCandidates([])
    setEdits({})
    try {
      const stream = await replayContinuationTask(id)
      await consumeStream(stream)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }

  const abortTask = async () => {
    if (!taskId) return
    try {
      await abortContinuationTask(taskId)
      setTaskStatus('cancelled')
      setRunning(false)
      await listContinuationTasks(workspace).then(setHistory).catch(() => {})
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }

  const openCommit = (candidate: ContinuationCandidate) => {
    const edit = edits[candidate.index] || { title: '', direction: '', excerpt: '' }
    setCommitRequest({ taskId, index: candidate.index, title: edit.title, content: '' })
    setCommitTarget('new_chapter')
    setCommitDraftMode('summary')
  }

  const buildCommitContent = () => {
    if (!commitRequest) return ''
    const edit = edits[commitRequest.index]
    const body = edit?.excerpt.trim() || edit?.direction.trim() || ''
    if (commitTarget === 'append') return body
    if (config.mode === 'excerpt') return body
    return commitDraftMode === 'summary'
      ? `${edit?.direction.trim() || ''}`
      : ''
  }

  const performCommit = async () => {
    if (!commitRequest) return
    setCommitting(true)
    setError('')
    try {
      await commitContinuation({
        taskId: commitRequest.taskId,
        index: commitRequest.index,
        title: commitRequest.title.trim() || t('continuation.defaultTitle'),
        content: buildCommitContent(),
        target: commitTarget,
      })
      setCommitRequest(null)
      if (onWorkspaceChanged) onWorkspaceChanged([workspace])
      await listContinuationTasks(workspace).then(setHistory).catch(() => {})
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
      throw err
    } finally {
      setCommitting(false)
    }
  }

  const performDelete = async () => {
    if (!deleteRequest) return
    setError('')
    try {
      await deleteContinuationTask(deleteRequest)
      setDeleteRequest(null)
      await listContinuationTasks(workspace).then(setHistory).catch(() => {})
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
      throw err
    }
  }

  const close = () => {
    if (running) {
      void abortTask()
    }
    onOpenChange(false)
  }

  const taskDone = taskStatus === 'done' || taskStatus === 'partially_done'
  const canCommit = (candidate: ContinuationCandidate) => {
    if (!taskDone || candidate.status !== 'done') return false
    const edit = edits[candidate.index]
    if (!edit) return false
    if (config.mode === 'excerpt') return edit.excerpt.trim() !== ''
    return edit.direction.trim() !== '' || edit.title.trim() !== ''
  }

  return (
    <>
      <Dialog open={open} onOpenChange={(next) => { if (!next) close() }}>
        <DialogContent
          className="nova-panel flex max-h-[85vh] w-[min(760px,calc(100vw-2rem))] max-w-[min(760px,calc(100vw-2rem))] flex-col rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0 text-[var(--nova-text)] shadow-[var(--nova-shadow)]"
          aria-describedby="continuation-desc"
        >
          <div className="flex items-center justify-between gap-2 border-b border-[var(--nova-border)] px-4 py-3">
            <div className="min-w-0">
              <DialogTitle className="flex items-center gap-1.5 text-sm font-semibold text-[var(--nova-text)]">
                <Sparkles className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
                {t('continuation.title')}
              </DialogTitle>
              <DialogDescription id="continuation-desc" className="mt-0.5 truncate text-[11px] text-[var(--nova-text-faint)]">
                {view === 'config' ? t('continuation.configDescription') : t('continuation.panelDescription')}
              </DialogDescription>
            </div>
            {view === 'panel' && taskStatus && (
              <div className="flex shrink-0 items-center gap-1.5">
                {running && <Loader2 className="h-3.5 w-3.5 animate-spin text-[var(--nova-text-muted)]" />}
                <span className="text-[11px] text-[var(--nova-text-muted)]">{t(`continuation.status.${taskStatus}`)}</span>
                {running && (
                  <Button type="button" size="xs" variant="ghost" className="h-6 gap-1 px-1.5 text-[10px] text-[var(--nova-text-muted)]" onClick={() => void abortTask()}>
                    <XCircle className="h-3 w-3" />
                    {t('continuation.abort')}
                  </Button>
                )}
              </div>
            )}
          </div>

          <div className="min-h-0 flex-1 overflow-y-auto px-4 py-4">
            {view === 'config' ? (
              <div className="space-y-4 text-xs">
                <div className="grid gap-3 md:grid-cols-2">
                  <div className="space-y-1.5">
                    <Label className="text-[11px] font-medium text-[var(--nova-text-faint)]">{t('continuation.anchor')}</Label>
                    <Select value={config.anchor} onValueChange={(value) => updateConfig('anchor', value)}>
                      <SelectTrigger size="sm" className="w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {chapterOptions.map(([path, title]) => (
                          <SelectItem key={path} value={path} className="text-xs">{title}</SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="space-y-1.5">
                    <Label className="text-[11px] font-medium text-[var(--nova-text-faint)]">{t('continuation.depth')}</Label>
                    <Select value={config.depth} onValueChange={(value) => updateConfig('depth', value as ContinuationDepth)}>
                      <SelectTrigger size="sm" className="w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="short" className="text-xs">{t('continuation.depthShort')}</SelectItem>
                        <SelectItem value="medium" className="text-xs">{t('continuation.depthMedium')}</SelectItem>
                        <SelectItem value="long" className="text-xs">{t('continuation.depthLong')}</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                </div>

                <div className="space-y-1.5">
                  <Label className="text-[11px] font-medium text-[var(--nova-text-faint)]">{t('continuation.mode')}</Label>
                  <RadioGroup value={config.mode} onValueChange={(value) => updateConfig('mode', value as ContinuationMode)} className="flex gap-3">
                    <Label className="flex cursor-pointer items-center gap-1.5 text-xs text-[var(--nova-text)]">
                      <RadioGroupItem value="direction" />
                      {t('continuation.modeDirection')}
                    </Label>
                    <Label className={`flex cursor-pointer items-center gap-1.5 text-xs ${config.depth === 'short' ? 'cursor-not-allowed opacity-40' : 'text-[var(--nova-text)]'}`}>
                      <RadioGroupItem value="excerpt" disabled={config.depth === 'short'} />
                      {t('continuation.modeExcerpt')}
                    </Label>
                  </RadioGroup>
                  {config.depth === 'short' && <div className="text-[10px] text-[var(--nova-text-faint)]">{t('continuation.modeShortHint')}</div>}
                </div>

                <div className="space-y-1.5">
                  <Label className="text-[11px] font-medium text-[var(--nova-text-faint)]">{t('continuation.styleTemplate')}</Label>
                  <Select
                    value={config.styleRefNames[0] || 'none'}
                    onValueChange={(value) => updateConfig('styleRefNames', value === 'none' ? [] : [value])}
                  >
                    <SelectTrigger size="sm" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="none" className="text-xs">{t('continuation.styleNone')}</SelectItem>
                      {styles.map((style) => (
                        <SelectItem key={style.name} value={style.name} className="text-xs">{style.name}</SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  {styles.length === 0 && (
                    <div className="text-[10px] leading-4 text-[var(--nova-text-faint)]">{t('continuation.styleEmpty')}</div>
                  )}
                </div>

                <div className="space-y-1.5">
                  <Label className="text-[11px] font-medium text-[var(--nova-text-faint)]">{t('continuation.customStyle')}</Label>
                  <Textarea value={config.customStyle} onChange={(event) => updateConfig('customStyle', event.target.value)} rows={2} className="text-xs" placeholder={t('continuation.customStylePlaceholder')} />
                </div>

                <div className="space-y-1.5">
                  <Label className="text-[11px] font-medium text-[var(--nova-text-faint)]">{t('continuation.preference')}</Label>
                  <Textarea value={config.preference} onChange={(event) => updateConfig('preference', event.target.value)} rows={2} className="text-xs" placeholder={t('continuation.preferencePlaceholder')} />
                </div>

                {error && <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2 text-[11px] text-[var(--nova-danger-text)]">{error}</div>}
              </div>
            ) : (
              <div className="space-y-3">
                {error && <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2 text-[11px] text-[var(--nova-danger-text)]">{error}</div>}
                {candidates.length === 0 && running && (
                  <div className="flex items-center justify-center gap-2 py-8 text-xs text-[var(--nova-text-muted)]">
                    <Loader2 className="h-4 w-4 animate-spin" />
                    {t('continuation.generating')}
                  </div>
                )}
                <div className="space-y-2">
                  {candidates.map((candidate) => (
                    <CandidateCard
                      key={candidate.index}
                      candidate={candidate}
                      edit={edits[candidate.index]}
                      mode={config.mode}
                      onEdit={(edit) => setEdits((current) => ({ ...current, [candidate.index]: edit }))}
                      onCommit={() => openCommit(candidate)}
                      canCommit={canCommit(candidate)}
                      commitLabel={t('continuation.commitButton')}
                    />
                  ))}
                </div>

                <div className="space-y-1.5 pt-2">
                  <div className="flex items-center gap-1.5 text-[11px] font-medium text-[var(--nova-text-faint)]">
                    <ChevronDown className="h-3 w-3" />
                    {t('continuation.history')}
                  </div>
                  {history.length === 0 && <div className="text-[10px] text-[var(--nova-text-faint)]">{t('continuation.historyEmpty')}</div>}
                  {history.map((task) => (
                    <div key={task.id} className="flex items-center gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2.5 py-1.5">
                      <button type="button" className="min-w-0 flex-1 truncate text-left text-[11px] text-[var(--nova-text)]" onClick={() => void replayTask(task.id)}>
                        {t('continuation.taskLabel', { id: task.id.slice(-8), status: t(`continuation.status.${task.status}`) })}
                      </button>
                      <button type="button" aria-label={t('continuation.deleteTask')} className="rounded p-1 text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]" onClick={() => setDeleteRequest(task.id)}>
                        <Trash2 className="h-3 w-3" />
                      </button>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>

          <div className="flex items-center justify-between gap-2 border-t border-[var(--nova-border)] px-4 py-3">
            <div className="flex items-center gap-1.5">
              {view === 'panel' && (
                <Button type="button" size="xs" variant="ghost" className="nova-nav-item gap-1 text-[var(--nova-text-faint)]" onClick={() => { setView('config'); setError('') }} disabled={running}>
                  <RotateCcw className="h-3.5 w-3.5" />
                  {t('continuation.reconfigure')}
                </Button>
              )}
            </div>
            <div className="flex items-center gap-2">
              <Button type="button" size="xs" variant="ghost" className="nova-nav-item text-[var(--nova-text-faint)]" onClick={close} disabled={committing}>
                {t('common.close')}
              </Button>
              {view === 'config' && (
                <Button type="button" size="xs" className="nova-nav-item gap-1" onClick={() => void startExplore()} disabled={running}>
                  {running ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Play className="h-3.5 w-3.5" />}
                  {t('continuation.start')}
                </Button>
              )}
              {view === 'panel' && taskDone && (
                <Button type="button" size="xs" variant="ghost" className="nova-nav-item gap-1 border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text)]" onClick={() => void replayTask(taskId)}>
                  <RefreshCw className="h-3.5 w-3.5" />
                  {t('continuation.reload')}
                </Button>
              )}
            </div>
          </div>
        </DialogContent>
      </Dialog>

      {commitRequest && (
        <ConfirmDialog
          open={true}
          onOpenChange={(next) => { if (!next && !committing) setCommitRequest(null) }}
          title={t('continuation.commitTitle')}
          description={t('continuation.commitDescription')}
          confirmLabel={t('continuation.commitButton')}
          tone="default"
          detailContent={
            <div className="space-y-2 text-xs">
              <div className="space-y-1">
                <div className="text-[11px] font-medium text-[var(--nova-text-faint)]">{t('continuation.commitTitleField')}</div>
                <Input
                  value={commitRequest.title}
                  onChange={(event) => setCommitRequest((current) => current ? { ...current, title: event.target.value } : null)}
                  className="h-8 text-xs"
                />
              </div>
              <div className="space-y-1">
                <div className="text-[11px] font-medium text-[var(--nova-text-faint)]">{t('continuation.commitTargetField')}</div>
                <Select value={commitTarget} onValueChange={(value) => setCommitTarget(value as ContinuationTarget)}>
                  <SelectTrigger size="sm" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="new_chapter" className="text-xs">{t('continuation.targetNewChapter')}</SelectItem>
                    <SelectItem value="append" className="text-xs">{t('continuation.targetAppend')}</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              {config.mode === 'direction' && commitTarget === 'new_chapter' && (
                <div className="space-y-1">
                  <div className="text-[11px] font-medium text-[var(--nova-text-faint)]">{t('continuation.commitDraftMode')}</div>
                  <RadioGroup value={commitDraftMode} onValueChange={(value) => setCommitDraftMode(value as 'summary' | 'empty')} className="flex gap-3">
                    <Label className="flex cursor-pointer items-center gap-1.5 text-xs text-[var(--nova-text)]">
                      <RadioGroupItem value="summary" />
                      {t('continuation.commitDraftSummary')}
                    </Label>
                    <Label className="flex cursor-pointer items-center gap-1.5 text-xs text-[var(--nova-text)]">
                      <RadioGroupItem value="empty" />
                      {t('continuation.commitDraftEmpty')}
                    </Label>
                  </RadioGroup>
                </div>
              )}
            </div>
          }
          onConfirm={performCommit}
        />
      )}

      {deleteRequest && (
        <ConfirmDialog
          open={true}
          onOpenChange={(next) => { if (!next) setDeleteRequest(null) }}
          title={t('continuation.deleteTaskTitle')}
          description={t('continuation.deleteTaskDescription')}
          confirmLabel={t('continuation.deleteTask')}
          tone="danger"
          onConfirm={performDelete}
        />
      )}
    </>
  )
}

function CandidateCard({
  candidate,
  edit,
  mode,
  onEdit,
  onCommit,
  canCommit,
  commitLabel,
}: {
  candidate: ContinuationCandidate
  edit: CandidateEdit | undefined
  mode: ContinuationMode
  onEdit: (edit: CandidateEdit) => void
  onCommit: () => void
  canCommit: boolean
  commitLabel: string
}) {
  const { t } = useTranslation()
  const expanded = candidate.status === 'done'
  return (
    <div className={`rounded-[var(--nova-radius)] border px-3 py-2.5 ${candidate.status === 'failed' ? 'border-[var(--nova-danger)]/30 bg-[var(--nova-danger-bg)]' : 'border-[var(--nova-border)] bg-[var(--nova-surface)]'}`}>
      <div className="flex items-center gap-2">
        <span className="text-[11px] font-medium text-[var(--nova-text-faint)]">{t('continuation.candidateLabel', { index: candidate.index + 1 })}</span>
        {candidate.status === 'generating' && <Loader2 className="h-3 w-3 animate-spin text-[var(--nova-text-muted)]" />}
        {candidate.status === 'done' && <CheckCircle2 className="h-3 w-3 text-[var(--nova-success)]" />}
        {candidate.status === 'failed' && <XCircle className="h-3 w-3 text-[var(--nova-danger-text)]" />}
        {candidate.status === 'failed' && <span className="min-w-0 flex-1 truncate text-[10px] text-[var(--nova-danger-text)]">{candidate.error || t('continuation.candidateFailed')}</span>}
        {candidate.status === 'done' && (
          <button type="button" className="nova-nav-item ml-auto shrink-0 gap-1 rounded border border-[var(--nova-border)] px-2 py-1 text-[10px] text-[var(--nova-text)] disabled:cursor-not-allowed disabled:opacity-40" onClick={onCommit} disabled={!canCommit}>
            <Send className="h-3 w-3" />
            {commitLabel}
          </button>
        )}
      </div>
      {expanded && (
        <div className="mt-2 space-y-2">
          <Input
            value={edit?.title || ''}
            onChange={(event) => onEdit({ title: event.target.value, direction: edit?.direction || '', excerpt: edit?.excerpt || '' })}
            className="h-8 text-xs"
            placeholder={t('continuation.candidateTitlePlaceholder')}
          />
          {edit && (
            <Textarea
              value={edit.direction}
              onChange={(event) => onEdit({ title: edit.title, direction: event.target.value, excerpt: edit.excerpt })}
              rows={2}
              className="text-xs"
              placeholder={t('continuation.candidateDirectionPlaceholder')}
            />
          )}
          {(mode === 'excerpt' || edit?.excerpt) && edit && (
            <Textarea
              value={edit.excerpt}
              onChange={(event) => onEdit({ title: edit.title, direction: edit.direction, excerpt: event.target.value })}
              rows={5}
              className="text-xs"
              placeholder={t('continuation.candidateExcerptPlaceholder')}
            />
          )}
        </div>
      )}
    </div>
  )
}

function readStoredConfig(fallbackAnchor: string): ContinuationConfig {
  try {
    const parsed = JSON.parse(window.localStorage.getItem(CONFIG_STORAGE_KEY) || 'null')
    if (parsed && typeof parsed === 'object') {
      return {
        anchor: typeof parsed.anchor === 'string' && parsed.anchor ? parsed.anchor : fallbackAnchor,
        depth: parsed.depth === 'short' || parsed.depth === 'long' ? parsed.depth : 'medium',
        mode: parsed.mode === 'direction' ? 'direction' : 'excerpt',
        styleRefNames: Array.isArray(parsed.styleRefNames) ? parsed.styleRefNames.filter((item: unknown) => typeof item === 'string') : [],
        customStyle: typeof parsed.customStyle === 'string' ? parsed.customStyle : '',
        preference: typeof parsed.preference === 'string' ? parsed.preference : '',
      }
    }
  } catch {
    // 忽略损坏的本地配置
  }
  return { ...DEFAULT_CONFIG, anchor: fallbackAnchor }
}
