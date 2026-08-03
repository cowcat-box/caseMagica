import { fetchAPI, jsonHeaders, parseSSEStream, readErrorMessage, requestJSON } from './client'
import type { BookCoverResult, BookMeta, BookRecord, BookSortMode, BookshelfResult, NovelImportResult, SSEEvent } from './types'

export type BookExportFormat = 'txt' | 'md' | 'zip'

export interface BookExportFile {
  filename: string
  blob: Blob
}

/** 导出内容勾选：不传任何字段时后端保持旧行为（导出全书章节正文）。 */
export interface BookExportSelection {
  chapters?: boolean
  groups?: 'latest' | 'all'
  outline?: boolean
  rules?: boolean
  progress?: boolean
  characterStates?: boolean
  ideas?: boolean
  meta?: boolean
}

export async function getBookshelf(): Promise<BookshelfResult> {
  const data = await requestJSON<Partial<BookshelfResult>>('/api/books')
  return {
    books: data.books || [],
    sort_mode: data.sort_mode === 'manual' ? 'manual' : 'recent',
  }
}

export async function getBooks(): Promise<BookRecord[]> {
  return (await getBookshelf()).books
}

export async function removeBook(path: string): Promise<{ message: string; workspace: string }> {
  return requestJSON('/api/books/remove', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ path }),
  })
}

export async function reorderBooks(paths: string[]): Promise<{ message: string }> {
  return requestJSON('/api/books/reorder', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ paths }),
  })
}

export async function setBookSortMode(mode: BookSortMode): Promise<{ message: string }> {
  return requestJSON('/api/books/sort-mode', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ mode }),
  })
}

export async function previewNovelImportStream(
  file: File,
  options: { sampleChars?: number; splitRegex?: string; splitStrategy?: string } = {},
): Promise<ReadableStream<SSEEvent>> {
  const form = new FormData()
  form.append('file', file)
  if (options.sampleChars !== undefined) form.append('sample_chars', String(options.sampleChars))
  if (options.splitRegex !== undefined) form.append('split_regex', options.splitRegex)
  if (options.splitStrategy) form.append('split_strategy', options.splitStrategy)
  const res = await fetchAPI('/api/books/import-novel/preview/stream', {
    method: 'POST',
    body: form,
  })
  if (!res.ok) {
    const data = await res.json().catch(() => ({}))
    throw new Error(data.error || `HTTP ${res.status}`)
  }
  if (!res.body) throw new Error('No response body')
  return parseSSEStream(res.body)
}

export async function importNovel(
  file: File,
  options: { bookTitle?: string; author?: string; description?: string; sampleChars?: number; splitRegex?: string; splitStrategy?: string } = {},
): Promise<NovelImportResult> {
  const form = new FormData()
  form.append('file', file)
  if (options.bookTitle) form.append('book_title', options.bookTitle)
  if (options.author) form.append('author', options.author)
  if (options.description) form.append('description', options.description)
  if (options.sampleChars !== undefined) form.append('sample_chars', String(options.sampleChars))
  if (options.splitRegex !== undefined) form.append('split_regex', options.splitRegex)
  if (options.splitStrategy) form.append('split_strategy', options.splitStrategy)
  return requestJSON('/api/books/import-novel', {
    method: 'POST',
    body: form,
  })
}

export async function createBook(title: string, author?: string, description?: string): Promise<{ workspace: string; book_meta: BookMeta }> {
  return requestJSON('/api/books/create', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ title, author: author ?? '', description: description ?? '' }),
  })
}

export async function getBookInfo(path: string): Promise<BookMeta> {
  return requestJSON(`/api/books/info?path=${encodeURIComponent(path)}`)
}

export async function updateBookInfo(path: string, title: string, author: string, description: string): Promise<BookMeta> {
  return requestJSON('/api/books/info', {
    method: 'PUT',
    headers: jsonHeaders,
    body: JSON.stringify({ path, title, author, description }),
  })
}

export function bookCoverURL(path: string, version?: string): string {
  const params = new URLSearchParams({ path })
  if (version) params.set('v', version)
  return `/api/books/cover?${params.toString()}`
}

export async function generateBookCover(input: {
  path: string
  imagePresetId?: string
  instruction?: string
  profileId?: string
}): Promise<BookCoverResult> {
  return requestJSON('/api/books/cover/generate', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({
      path: input.path,
      image_preset_id: input.imagePresetId || '',
      instruction: input.instruction || '',
      profile_id: input.profileId || '',
    }),
  })
}

export async function uploadBookCover(path: string, file: File): Promise<BookCoverResult> {
  const form = new FormData()
  form.append('path', path)
  form.append('file', file)
  return requestJSON('/api/books/cover/upload', {
    method: 'POST',
    body: form,
  })
}

export async function exportBook(input: { path: string; format: BookExportFormat; selection?: BookExportSelection }): Promise<BookExportFile> {
  const params = new URLSearchParams({ path: input.path, format: input.format })
  const selection = input.selection
  if (selection) {
    if (selection.chapters) params.set('chapters', '1')
    if (selection.groups) params.set('groups', selection.groups)
    if (selection.outline) params.set('outline', '1')
    if (selection.rules) params.set('rules', '1')
    if (selection.progress) params.set('progress', '1')
    if (selection.characterStates) params.set('character_states', '1')
    if (selection.ideas) params.set('ideas', '1')
    if (selection.meta) params.set('meta', '1')
  }
  const res = await fetchAPI(`/api/books/export?${params.toString()}`)
  if (!res.ok) {
    throw new Error(await readErrorMessage(res))
  }
  const filename = filenameFromContentDisposition(res.headers.get('Content-Disposition')) || `book.${input.format}`
  return { filename, blob: await res.blob() }
}

export function downloadBookExport(file: BookExportFile) {
  const href = URL.createObjectURL(file.blob)
  const link = document.createElement('a')
  link.href = href
  link.download = file.filename
  document.body.appendChild(link)
  link.click()
  link.remove()
  window.setTimeout(() => URL.revokeObjectURL(href), 0)
}

export type SettingsKind = 'outline' | 'rules' | 'progress' | 'character_states' | 'ideas' | 'chapter_group'

export interface SettingsImportPreview {
  kind: SettingsKind
  target_path?: string
}

export interface SettingsImportResult {
  target_path: string
  backup_path?: string
}

/** 识别书籍设定材料类型并预览目标落位（不写入）。 */
export async function identifySettingsImport(workspace: string, file: File, presetKind?: SettingsKind): Promise<SettingsImportPreview> {
  const form = new FormData()
  form.append('path', workspace)
  if (presetKind) form.append('kind', presetKind)
  form.append('file', file)
  return requestJSON<SettingsImportPreview>('/api/books/settings/identify', {
    method: 'POST',
    body: form,
  })
}

/** 导入书籍设定材料（写入前自动备份原文件）。 */
export async function importSettingsFile(workspace: string, file: File, kind: SettingsKind): Promise<SettingsImportResult> {
  const form = new FormData()
  form.append('path', workspace)
  form.append('kind', kind)
  form.append('file', file)
  return requestJSON<SettingsImportResult>('/api/books/settings/import', {
    method: 'POST',
    body: form,
  })
}

/** 下载单个设定文件。 */
export async function downloadSettingsFile(workspace: string, filePath: string) {
  const params = new URLSearchParams({ path: workspace, file: filePath })
  const res = await fetchAPI(`/api/books/settings/export?${params.toString()}`)
  if (!res.ok) {
    throw new Error(await readErrorMessage(res))
  }
  const filename = filenameFromContentDisposition(res.headers.get('Content-Disposition')) || filePath.split('/').pop() || 'setting.md'
  downloadBookExport({ filename, blob: await res.blob() })
}

/** 打包下载全部章节组细纲。 */
export async function downloadChapterGroups(workspace: string) {
  const params = new URLSearchParams({ path: workspace })
  const res = await fetchAPI(`/api/books/settings/export/groups?${params.toString()}`)
  if (!res.ok) {
    throw new Error(await readErrorMessage(res))
  }
  const filename = filenameFromContentDisposition(res.headers.get('Content-Disposition')) || 'chapter-groups.zip'
  downloadBookExport({ filename, blob: await res.blob() })
}

function filenameFromContentDisposition(header: string | null): string {
  if (!header) return ''
  const encoded = /filename\*=UTF-8''([^;]+)/i.exec(header)
  if (encoded?.[1]) {
    try {
      return decodeURIComponent(encoded[1])
    } catch {
      return encoded[1]
    }
  }
  const quoted = /filename="([^"]+)"/i.exec(header)
  if (quoted?.[1]) return quoted[1]
  const plain = /filename=([^;]+)/i.exec(header)
  return plain?.[1]?.trim() || ''
}
