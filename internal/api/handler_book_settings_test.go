package api

import (
	"archive/zip"
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"
)

func settingsImportBody(t *testing.T, path, filename, content, kind string) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("path", path); err != nil {
		t.Fatal(err)
	}
	if kind != "" {
		if err := writer.WriteField("kind", kind); err != nil {
			t.Fatal(err)
		}
	}
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes(), writer.FormDataContentType()
}

func TestSettingsImportExportAPI(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	workspace := application.Workspace()

	if err := application.BookService().Create("setting/outline.md", "file", "旧大纲"); err != nil {
		t.Fatal(err)
	}
	if err := application.BookService().Create("setting/chapter-groups/group1-第一组.md", "file", "# 第一组\n\n前五章。"); err != nil {
		t.Fatal(err)
	}

	body, contentType := settingsImportBody(t, workspace, "outline.md", "# 新大纲\n\n内容。", "")
	resp := ut.PerformRequest(
		server.engine.Engine,
		http.MethodPost,
		"/api/books/settings/import",
		&ut.Body{Body: bytes.NewReader(body), Len: len(body)},
		ut.Header{Key: "Content-Type", Value: contentType},
	)
	if resp.Code != http.StatusOK {
		t.Fatalf("import status = %d body=%s", resp.Code, resp.Body.String())
	}
	var result struct {
		TargetPath string `json:"target_path"`
		BackupPath string `json:"backup_path"`
	}
	decodeResponse(t, resp.Body.Bytes(), &result)
	if result.TargetPath != "setting/outline.md" || result.BackupPath == "" {
		t.Fatalf("import result mismatch: %#v", result)
	}
	content, err := application.BookService().ReadFile("setting/outline.md")
	if err != nil {
		t.Fatal(err)
	}
	if content != "# 新大纲\n\n内容。" {
		t.Fatalf("content = %q", content)
	}
	backup, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(result.BackupPath)))
	if err != nil {
		t.Fatalf("读取备份失败: %v", err)
	}
	if string(backup) != "旧大纲" {
		t.Fatalf("backup content = %q", backup)
	}

	exportResp := ut.PerformRequest(
		server.engine.Engine,
		http.MethodGet,
		"/api/books/settings/export?path="+url.QueryEscape(workspace)+"&file="+url.QueryEscape("setting/outline.md"),
		nil,
	)
	if exportResp.Code != http.StatusOK {
		t.Fatalf("export status = %d body=%s", exportResp.Code, exportResp.Body.String())
	}
	if exportResp.Body.String() != "# 新大纲\n\n内容。" {
		t.Fatalf("export body = %q", exportResp.Body.String())
	}
}

func TestSettingsImportUnknownKindRequiresManualChoice(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")

	body, contentType := settingsImportBody(t, application.Workspace(), "材料.md", "没有可识别关键词的内容", "")
	resp := ut.PerformRequest(
		server.engine.Engine,
		http.MethodPost,
		"/api/books/settings/import",
		&ut.Body{Body: bytes.NewReader(body), Len: len(body)},
		ut.Header{Key: "Content-Type", Value: contentType},
	)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "无法识别材料类型") {
		t.Fatalf("body = %q", resp.Body.String())
	}

	body, contentType = settingsImportBody(t, application.Workspace(), "材料.md", "手工指定", "outline")
	resp = ut.PerformRequest(
		server.engine.Engine,
		http.MethodPost,
		"/api/books/settings/import",
		&ut.Body{Body: bytes.NewReader(body), Len: len(body)},
		ut.Header{Key: "Content-Type", Value: contentType},
	)
	if resp.Code != http.StatusOK {
		t.Fatalf("manual kind import status = %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestSettingsExportGroupsAPI(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	workspace := application.Workspace()
	if err := application.BookService().Create("setting/chapter-groups/group1-第一组.md", "file", "# 第一组"); err != nil {
		t.Fatal(err)
	}

	resp := ut.PerformRequest(
		server.engine.Engine,
		http.MethodGet,
		"/api/books/settings/export/groups?path="+url.QueryEscape(workspace),
		nil,
	)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", resp.Code, resp.Body.String())
	}
	reader, err := zip.NewReader(bytes.NewReader(resp.Body.Bytes()), int64(resp.Body.Len()))
	if err != nil {
		t.Fatalf("zip parse failed: %v", err)
	}
	found := false
	for _, file := range reader.File {
		if file.Name != "setting/chapter-groups/group1-第一组.md" {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != "# 第一组" {
			t.Fatalf("content = %q", content)
		}
		found = true
	}
	if !found {
		t.Fatalf("zip missing group file")
	}
}
