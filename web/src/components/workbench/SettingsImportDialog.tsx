import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckCircle2, Upload } from 'lucide-react'
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { identifySettingsImport, importSettingsFile, type SettingsImportPreview, type SettingsKind } from '@/lib/api-client/books'

const KIND_OPTIONS: { value: SettingsKind; labelKey: string }[] = [
  { value: 'outline', labelKey: 'planning.outlineTab' },
  { value: 'rules', labelKey: 'planning.creatorRulesTab' },
  { value: 'progress', labelKey: 'planning.writingProgressTab' },
  { value: 'character_states', labelKey: 'planning.characterStates' },
  { value: 'ideas', labelKey: 'planning.ideas' },
  { value: 'chapter_group', labelKey: 'planning.chapterGroupTab' },
]

interface SettingsImportDialogProps {
  open: boolean
  workspace: string
  presetKind?: SettingsKind
  onOpenChange: (open: boolean) => void
  onImported?: () => void
}

export function SettingsImportDialog({ open, workspace, presetKind, onOpenChange, onImported }: SettingsImportDialogProps) {
  const { t } = useTranslation()
  const inputRef = useRef<HTMLInputElement | null>(null)
  const [file, setFile] = useState<File | null>(null)
  const [preview, setPreview] = useState<SettingsImportPreview | null>(null)
  const [manualKind, setManualKind] = useState<SettingsKind>('outline')
  const [identifying, setIdentifying] = useState(false)
  const [importing, setImporting] = useState(false)
  const [done, setDone] = useState(false)
  const [error, setError] = useState('')

  const reset = () => {
    setFile(null)
    setPreview(null)
    setManualKind('outline')
    setIdentifying(false)
    setImporting(false)
    setDone(false)
    setError('')
  }

  const handleOpenChange = (next: boolean) => {
    if (!next && !identifying && !importing) {
      reset()
      onOpenChange(false)
    }
  }

  const handleFileSelected = async (selected: File | undefined) => {
    if (!selected) return
    setFile(selected)
    setError('')
    setDone(false)
    setIdentifying(true)
    setPreview(null)
    try {
      setPreview(await identifySettingsImport(workspace, selected, presetKind))
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setIdentifying(false)
    }
  }

  const effectiveKind = preview?.kind || manualKind
  const needsManualKind = Boolean(file) && !identifying && (!preview || !preview.kind)
  const canImport = Boolean(file) && Boolean(effectiveKind) && !identifying && !importing

  const handleImport = async () => {
    if (!file || !canImport) return
    setImporting(true)
    setError('')
    try {
      await importSettingsFile(workspace, file, effectiveKind)
      setDone(true)
      onImported?.()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setImporting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent
        className="nova-panel w-[min(520px,calc(100vw-2rem))] max-w-[min(520px,calc(100vw-2rem))] rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0 text-[var(--nova-text)] shadow-[var(--nova-shadow)]"
        aria-describedby="settings-import-desc"
      >
        <div className="border-b border-[var(--nova-border)] px-4 py-3">
          <DialogTitle className="text-sm font-semibold text-[var(--nova-text)]">{t('planning.settingsImportTitle')}</DialogTitle>
          <DialogDescription id="settings-import-desc" className="mt-1 text-xs text-[var(--nova-text-faint)]">
            {t('planning.settingsImportDescription')}
          </DialogDescription>
        </div>
        <div className="space-y-4 px-4 py-4 text-xs">
          <input
            ref={inputRef}
            type="file"
            accept=".md,.txt,text/markdown,text/plain"
            className="hidden"
            onChange={(event) => void handleFileSelected(event.target.files?.[0])}
          />
          <div className="flex min-w-0 items-center gap-2">
            <Button type="button" size="xs" variant="ghost" className="nova-nav-item border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text)]" onClick={() => inputRef.current?.click()} disabled={identifying || importing}>
              <Upload className="h-3.5 w-3.5" />
              {t('planning.settingsImportChooseFile')}
            </Button>
            <div className="min-w-0 flex-1 truncate text-[var(--nova-text-faint)]">{file ? file.name : t('planning.settingsImportNoFile')}</div>
            {identifying && <span className="shrink-0 text-[var(--nova-text-muted)]">{t('planning.settingsImportIdentifying')}</span>}
          </div>

          {needsManualKind && (
            <div className="space-y-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2">
              <div className="text-[11px] font-medium text-[var(--nova-text)]">{t('planning.settingsImportKindUnknown')}</div>
              <Select value={manualKind} onValueChange={(value) => setManualKind(value as SettingsKind)}>
                <SelectTrigger size="sm" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {KIND_OPTIONS.map((option) => (
                    <SelectItem key={option.value} value={option.value} className="text-xs">{t(option.labelKey)}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          )}

          {preview?.kind && (
            <div className="space-y-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2">
              <div className="flex items-center gap-1.5 text-[11px] font-medium text-[var(--nova-text)]">
                <CheckCircle2 className="h-3.5 w-3.5 text-[var(--nova-success)]" />
                {t(`planning.settingsKind.${preview.kind}`)}
              </div>
              <div className="text-[11px] text-[var(--nova-text-muted)]">
                {t('planning.settingsImportTarget')}<span className="ml-1 font-mono text-[var(--nova-text)]">{preview.target_path || '-'}</span>
              </div>
              <div className="text-[10px] text-[var(--nova-text-faint)]">{t('planning.settingsImportBackupHint')}</div>
            </div>
          )}

          {done && <div className="rounded-[var(--nova-radius)] border border-[var(--nova-success)]/25 bg-[var(--nova-success-bg)] px-3 py-2 text-[11px] text-[var(--nova-success)]">{t('planning.settingsImportDone')}</div>}
          {error && <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2 text-[11px] text-[var(--nova-danger-text)]">{error}</div>}
        </div>
        <div className="flex items-center justify-end gap-2 border-t border-[var(--nova-border)] px-4 py-3">
          <Button type="button" size="xs" variant="ghost" className="nova-nav-item text-[var(--nova-text-faint)]" onClick={() => handleOpenChange(false)} disabled={importing}>
            {done ? t('common.close') : t('common.cancel')}
          </Button>
          {!done && (
            <Button type="button" size="xs" className="nova-nav-item" onClick={() => void handleImport()} disabled={!canImport} data-testid="settings-import-confirm">
              {importing ? t('planning.settingsImporting') : t('planning.settingsImportButton')}
            </Button>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}
