package app

import (
	"casemagica/internal/book"
)

// SettingsImportRequest 描述一次书籍设定材料导入。
type SettingsImportRequest struct {
	Path    string            `json:"path"`
	Kind    book.SettingsKind `json:"kind"`
	Content []byte            `json:"content"`
}

// SettingsImportResult 描述材料导入的落位与备份信息。
type SettingsImportResult struct {
	TargetPath string `json:"target_path"`
	BackupPath string `json:"backup_path,omitempty"`
}

// SettingsFileExport 描述单个设定文件的导出结果。
type SettingsFileExport struct {
	Filename    string
	ContentType string
	Data        []byte
}

// IdentifySettingsKind 根据文件名与内容启发式识别材料类型（供导入确认弹窗使用）。
func (a *App) IdentifySettingsKind(filename, content string) book.SettingsKind {
	return book.IdentifySettingsKind(filename, content)
}

// IdentifySettingsImport 识别材料类型并预览目标落位（不写入任何内容）。
func (a *App) IdentifySettingsImport(path, filename, content string, presetKind book.SettingsKind) (book.SettingsImportPreview, error) {
	absPath, err := validateBookWorkspacePath(path)
	if err != nil {
		return book.SettingsImportPreview{}, err
	}
	return book.NewService(absPath).IdentifySettingsImport(filename, content, presetKind)
}

// ImportSettingsFile 将材料写入书籍对应设定文件并返回备份信息。
func (a *App) ImportSettingsFile(req SettingsImportRequest) (SettingsImportResult, error) {
	absPath, err := validateBookWorkspacePath(req.Path)
	if err != nil {
		return SettingsImportResult{}, err
	}
	result, err := book.NewService(absPath).ImportSettingsFile(req.Content, req.Kind)
	if err != nil {
		return SettingsImportResult{}, err
	}
	return SettingsImportResult{
		TargetPath: result.TargetPath,
		BackupPath: result.BackupPath,
	}, nil
}

// ExportSettingsFile 导出单个设定文件（白名单校验）。
func (a *App) ExportSettingsFile(path, rel string) (SettingsFileExport, error) {
	absPath, err := validateBookWorkspacePath(path)
	if err != nil {
		return SettingsFileExport{}, err
	}
	filename, content, err := book.NewService(absPath).SettingsExportSingle(rel)
	if err != nil {
		return SettingsFileExport{}, err
	}
	return SettingsFileExport{
		Filename:    filename,
		ContentType: "text/markdown; charset=utf-8",
		Data:        content,
	}, nil
}

// ExportSettingsGroups 打包导出全部章节组细纲。
func (a *App) ExportSettingsGroups(path string) (SettingsFileExport, error) {
	absPath, err := validateBookWorkspacePath(path)
	if err != nil {
		return SettingsFileExport{}, err
	}
	data, err := book.NewService(absPath).SettingsExportGroups()
	if err != nil {
		return SettingsFileExport{}, err
	}
	return SettingsFileExport{
		Filename:    "chapter-groups.zip",
		ContentType: "application/zip",
		Data:        data,
	}, nil
}
