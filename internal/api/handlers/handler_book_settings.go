package handlers

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	novaApp "casemagica/internal/app"
	"casemagica/internal/book"
)

const MaxSettingsImportBytes = 64 * 1024 * 1024

// HandleBookSettingsIdentify POST /api/books/settings/identify — 识别材料类型并预览目标落位。
// 提供 kind 参数时可强制指定类型（用于从具体设定项发起导入）。
func (h *Handlers) HandleBookSettingsIdentify(ctx context.Context, c *app.RequestContext) {
	path := strings.TrimSpace(string(c.FormValue("path")))
	if path == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.books.pathRequired")
		return
	}
	presetKind := book.SettingsKind(strings.TrimSpace(string(c.FormValue("kind"))))
	fileHeader, err := c.FormFile("file")
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.books.settingsImportFileRequired")
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.books.settingsImportReadFailed", "detail", err.Error())
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, MaxSettingsImportBytes+1))
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.books.settingsImportReadFailed", "detail", err.Error())
		return
	}
	preview, err := h.app.IdentifySettingsImport(path, fileHeader.Filename, string(data), presetKind)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, preview)
}

// HandleBookSettingsImport POST /api/books/settings/import — 导入书籍设定材料文件。
// 识别类型可由文件名/内容自动推断；识别失败时必须提供 kind 参数（手动选择兜底）。
func (h *Handlers) HandleBookSettingsImport(ctx context.Context, c *app.RequestContext) {
	path := strings.TrimSpace(string(c.FormValue("path")))
	if path == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.books.pathRequired")
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.books.settingsImportFileRequired")
		return
	}
	if fileHeader.Size > MaxSettingsImportBytes {
		writeErrorKey(c, consts.StatusBadRequest, "api.books.settingsImportTooLarge")
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.books.settingsImportReadFailed", "detail", err.Error())
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, MaxSettingsImportBytes+1))
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.books.settingsImportReadFailed", "detail", err.Error())
		return
	}
	if int64(len(data)) > MaxSettingsImportBytes {
		writeErrorKey(c, consts.StatusBadRequest, "api.books.settingsImportTooLarge")
		return
	}
	ext := strings.ToLower(strings.TrimPrefix(filepathExt(fileHeader.Filename), "."))
	if ext != "md" && ext != "txt" {
		writeErrorKey(c, consts.StatusBadRequest, "api.books.settingsImportTypeInvalid")
		return
	}

	kind := book.SettingsKind(strings.TrimSpace(string(c.FormValue("kind"))))
	if kind == "" {
		kind = h.app.IdentifySettingsKind(fileHeader.Filename, string(data))
	}
	if kind == book.SettingsKindUnknown {
		writeErrorKey(c, consts.StatusBadRequest, "api.books.settingsImportKindUnknown")
		return
	}

	result, err := h.app.ImportSettingsFile(novaApp.SettingsImportRequest{
		Path:    path,
		Kind:    kind,
		Content: data,
	})
	if err != nil {
		switch {
		case errors.Is(err, book.ErrSettingsImportUnsupportedKind):
			writeErrorKey(c, consts.StatusBadRequest, "api.books.settingsImportKindInvalid")
		default:
			writeError(c, consts.StatusBadRequest, err.Error())
		}
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

// HandleBookSettingsExport GET /api/books/settings/export?path=...&file=... — 导出单个设定文件。
func (h *Handlers) HandleBookSettingsExport(ctx context.Context, c *app.RequestContext) {
	path := strings.TrimSpace(string(c.Query("path")))
	file := strings.TrimSpace(string(c.Query("file")))
	if path == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.books.pathQueryRequired")
		return
	}
	if file == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.books.settingsExportFileRequired")
		return
	}
	result, err := h.app.ExportSettingsFile(path, file)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	c.Response.Header.Set("Content-Disposition", attachmentContentDisposition(result.Filename))
	c.Response.Header.Set("Cache-Control", "no-store")
	c.Data(consts.StatusOK, result.ContentType, result.Data)
}

// HandleBookSettingsExportGroups GET /api/books/settings/export/groups?path=... — 打包导出全部细纲。
func (h *Handlers) HandleBookSettingsExportGroups(ctx context.Context, c *app.RequestContext) {
	path := strings.TrimSpace(string(c.Query("path")))
	if path == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.books.pathQueryRequired")
		return
	}
	result, err := h.app.ExportSettingsGroups(path)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	c.Response.Header.Set("Content-Disposition", attachmentContentDisposition(result.Filename))
	c.Response.Header.Set("Cache-Control", "no-store")
	c.Data(consts.StatusOK, result.ContentType, result.Data)
}

func filepathExt(filename string) string {
	base := filename
	if idx := strings.LastIndexAny(base, "/\\"); idx >= 0 {
		base = base[idx+1:]
	}
	if idx := strings.LastIndex(base, "."); idx >= 0 {
		return base[idx:]
	}
	return ""
}
