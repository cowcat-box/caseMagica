import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Download } from 'lucide-react'
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { downloadBookExport, exportBook, type BookExportFormat } from '@/lib/api-client/books'
import { cn } from '@/lib/utils'

type ChapterGroupsChoice = 'none' | 'latest' | 'all'

interface ExportSelection {
  chapters: boolean
  groups: ChapterGroupsChoice
  outline: boolean
  rules: boolean
  progress: boolean
  characterStates: boolean
  ideas: boolean
  meta: boolean
}

const EMPTY_SELECTION: ExportSelection = {
  chapters: true,
  groups: 'none',
  outline: false,
  rules: false,
  progress: false,
  characterStates: false,
  ideas: false,
  meta: false,
}

function selectionCount(selection: ExportSelection): number {
  let count = 0
  if (selection.chapters) count++
  if (selection.groups !== 'none') count++
  if (selection.outline) count++
  if (selection.rules) count++
  if (selection.progress) count++
  if (selection.characterStates) count++
  if (selection.ideas) count++
  if (selection.meta) count++
  return count
}

interface ExportDialogProps {
  open: boolean
  workspace: string
  onOpenChange: (open: boolean) => void
}

export function ExportDialog({ open, workspace, onOpenChange }: ExportDialogProps) {
  const { t } = useTranslation()
  const [selection, setSelection] = useState<ExportSelection>(EMPTY_SELECTION)
  const [format, setFormat] = useState<BookExportFormat>('zip')
  const [exporting, setExporting] = useState(false)
  const [error, setError] = useState('')

  const singleFileOnly = selectionCount(selection) === 1 && !selection.chapters && selection.groups === 'none'
  const formatOptions: BookExportFormat[] = selection.chapters && selectionCount(selection) === 1
    ? ['txt', 'md']
    : singleFileOnly
      ? ['md']
      : ['zip']

  const setField = <K extends keyof ExportSelection>(key: K, value: ExportSelection[K]) => {
    setSelection((prev) => ({ ...prev, [key]: value }))
  }

  const handleExport = async () => {
    if (selectionCount(selection) === 0 || exporting) return
    setExporting(true)
    setError('')
    try {
      const file = await exportBook({
        path: workspace,
        format,
        selection: {
          chapters: selection.chapters || undefined,
          groups: selection.groups === 'none' ? undefined : selection.groups,
          outline: selection.outline || undefined,
          rules: selection.rules || undefined,
          progress: selection.progress || undefined,
          characterStates: selection.characterStates || undefined,
          ideas: selection.ideas || undefined,
          meta: selection.meta || undefined,
        },
      })
      downloadBookExport(file)
      onOpenChange(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setExporting(false)
    }
  }

  const checkboxRow = (
    key: keyof ExportSelection,
    label: string,
    hint: string | undefined,
    checked: boolean,
    onChange: (value: boolean) => void,
  ) => (
    <label className="flex cursor-pointer items-start gap-2 rounded-[var(--nova-radius)] px-1 py-1.5 hover:bg-[var(--nova-surface)]">
      <Checkbox size="sm" checked={checked} onCheckedChange={(value) => onChange(Boolean(value))} data-testid={`export-${key}`} />
      <span className="min-w-0 flex-1">
        <span className="block text-xs font-medium text-[var(--nova-text)]">{label}</span>
        {hint && <span className="block text-[11px] text-[var(--nova-text-faint)]">{hint}</span>}
      </span>
    </label>
  )

  return (
    <Dialog open={open} onOpenChange={(next) => { if (!next && !exporting) onOpenChange(next) }}>
      <DialogContent
        className="nova-panel w-[min(480px,calc(100vw-2rem))] max-w-[min(480px,calc(100vw-2rem))] rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0 text-[var(--nova-text)] shadow-[var(--nova-shadow)]"
        aria-describedby="book-export-desc"
      >
        <div className="border-b border-[var(--nova-border)] px-4 py-3">
          <DialogTitle className="text-sm font-semibold text-[var(--nova-text)]">{t('planning.exportTitle')}</DialogTitle>
          <DialogDescription id="book-export-desc" className="mt-1 text-xs text-[var(--nova-text-faint)]">
            {t('planning.exportDescription')}
          </DialogDescription>
        </div>
        <div className="space-y-3 px-4 py-4 text-xs">
          <div className="space-y-1">
            <div className="px-1 text-[11px] font-medium text-[var(--nova-text-faint)]">{t('planning.exportContent')}</div>
            {checkboxRow('chapters', t('planning.exportChapters'), t('planning.exportChaptersHint'), selection.chapters, (value) => {
              setField('chapters', value)
              if (!value && format === 'txt') setFormat('zip')
            })}
            <div className="flex items-center gap-2 rounded-[var(--nova-radius)] px-1 py-1.5">
              <Checkbox size="sm" checked={selection.groups !== 'none'} onCheckedChange={(value) => setField('groups', value ? 'latest' : 'none')} data-testid="export-groups" />
              <span className="min-w-0 flex-1">
                <span className="block text-xs font-medium text-[var(--nova-text)]">{t('planning.exportGroups')}</span>
                <span className="block text-[11px] text-[var(--nova-text-faint)]">{t('planning.exportGroupsHint')}</span>
              </span>
              <Select
                value={selection.groups}
                onValueChange={(value) => setField('groups', value as ChapterGroupsChoice)}
                disabled={selection.groups === 'none'}
              >
                <SelectTrigger size="sm" className="w-36" data-testid="export-groups-range">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="latest">{t('planning.exportGroupsLatest')}</SelectItem>
                  <SelectItem value="all">{t('planning.exportGroupsAll')}</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>

          <div className="space-y-1">
            <div className="px-1 text-[11px] font-medium text-[var(--nova-text-faint)]">{t('planning.exportSettings')}</div>
            {checkboxRow('outline', t('planning.outline'), 'setting/outline.md', selection.outline, (value) => setField('outline', value))}
            {checkboxRow('rules', t('planning.creatorRules'), 'CREATOR.md', selection.rules, (value) => setField('rules', value))}
            {checkboxRow('progress', t('planning.writingProgress'), 'setting/progress.md', selection.progress, (value) => setField('progress', value))}
            {checkboxRow('characterStates', t('planning.characterStates'), 'setting/character-states.md', selection.characterStates, (value) => setField('characterStates', value))}
            {checkboxRow('ideas', t('planning.ideas'), 'ideas.md', selection.ideas, (value) => setField('ideas', value))}
            {checkboxRow('meta', t('planning.exportMeta'), 'book.json', selection.meta, (value) => setField('meta', value))}
          </div>

          <div className="flex items-center justify-between gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2">
            <span className="text-[11px] font-medium text-[var(--nova-text-faint)]">{t('planning.exportFormat')}</span>
            <div className="flex items-center gap-1">
              {formatOptions.map((option) => (
                <Button
                  key={option}
                  type="button"
                  size="xs"
                  variant="ghost"
                  className={cn('nova-nav-item border', format === option ? 'border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text)]' : 'border-transparent text-[var(--nova-text-faint)]')}
                  onClick={() => setFormat(option)}
                  data-testid={`export-format-${option}`}
                >
                  {option === 'txt' ? 'TXT' : option === 'md' ? 'MD' : 'ZIP'}
                </Button>
              ))}
            </div>
          </div>

          {error && <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2 text-[11px] text-[var(--nova-danger-text)]">{error}</div>}
        </div>
        <div className="flex items-center justify-end gap-2 border-t border-[var(--nova-border)] px-4 py-3">
          <Button type="button" size="xs" variant="ghost" className="nova-nav-item text-[var(--nova-text-faint)]" onClick={() => onOpenChange(false)} disabled={exporting}>
            {t('common.cancel')}
          </Button>
          <Button type="button" size="xs" className="nova-nav-item" onClick={() => void handleExport()} disabled={selectionCount(selection) === 0 || exporting} data-testid="export-confirm">
            <Download className="h-3.5 w-3.5" />
            {exporting ? t('planning.exporting') : t('planning.exportButton')}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
