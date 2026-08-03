package continuation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"casemagica/internal/book"
	"casemagica/internal/workspacechange"
)

// Committer 负责候选提交落盘：新建章节或追加章末，均走 workspacechange 原子写与 revision 校验。
type Committer struct {
	bookService   *book.Service
	changeService *workspacechange.Service
}

// NewCommitter 创建提交器。
func NewCommitter(bookService *book.Service, changeService *workspacechange.Service) *Committer {
	return &Committer{bookService: bookService, changeService: changeService}
}

// Commit 提交候选到目标位置。
func (c *Committer) Commit(request CommitRequest, task *Task) (CommitResult, error) {
	candidate, ok := task.Candidate(request.Index)
	if !ok {
		return CommitResult{}, fmt.Errorf("候选不存在: index=%d", request.Index)
	}
	title := strings.TrimSpace(request.Title)
	if title == "" {
		title = strings.TrimSpace(candidate.Title)
	}
	switch Target(request.Target) {
	case TargetNewChapter:
		return c.commitNewChapter(title, request.Content)
	case TargetAppend:
		return c.commitAppend(task, title, request.Content)
	default:
		return CommitResult{}, fmt.Errorf("不支持的提交目标: %s", request.Target)
	}
}

func (c *Committer) commitNewChapter(title, content string) (CommitResult, error) {
	rel, err := c.bookService.NextChapterFilename(title)
	if err != nil {
		return CommitResult{}, err
	}
	content = strings.TrimSpace(content)
	if content == "" && strings.TrimSpace(title) != "" {
		content = title
	}
	_, err = c.changeService.ReplaceFile(context.Background(), workspacechange.ReplaceFileRequest{
		Path:         rel,
		Content:      content,
		BaseRevision: "missing", // 新文件创建：base revision 为缺失标记
		Metadata:     continuationChangeMetadata(),
	})
	if err != nil {
		return CommitResult{}, fmt.Errorf("写入新章节失败: %w", err)
	}
	return CommitResult{Path: rel}, nil
}

func (c *Committer) commitAppend(task *Task, title, content string) (CommitResult, error) {
	anchor := strings.TrimSpace(task.Request.AnchorChapter)
	if anchor == "" {
		return CommitResult{}, fmt.Errorf("追加目标需要锚点章节")
	}
	before, revision, err := c.bookService.ReadFileWithRevision(anchor)
	if err != nil {
		return CommitResult{}, fmt.Errorf("读取锚点章节失败: %w", err)
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return CommitResult{}, fmt.Errorf("追加内容为空")
	}
	if strings.TrimSpace(before) == "" {
		// 空文件直接整体写入
		_, err := c.changeService.ReplaceFile(context.Background(), workspacechange.ReplaceFileRequest{
			Path:         anchor,
			Content:      strings.TrimSpace(before) + "\n\n" + content,
			BaseRevision: revision,
			Metadata:     continuationChangeMetadata(),
		})
		if err != nil {
			return CommitResult{}, fmt.Errorf("追加正文失败: %w", err)
		}
		return CommitResult{Path: anchor}, nil
	}
	tail := appendTailAnchor(before)
	_, err = c.changeService.ApplyEdits(context.Background(), workspacechange.ApplyEditsRequest{
		Path:         anchor,
		BaseRevision: revision,
		Edits: []workspacechange.TextEdit{{
			ID:        "continuation-append",
			OldString: tail,
			NewString: tail + "\n\n" + content,
		}},
		Metadata: continuationChangeMetadata(),
	})
	if err != nil {
		return CommitResult{}, fmt.Errorf("追加正文失败: %w", err)
	}
	return CommitResult{Path: anchor}, nil
}

// appendTailAnchor 取文件末尾一段唯一文本作为追加锚点（最多 60 字，跨行保留）。
func appendTailAnchor(content string) string {
	runes := []rune(content)
	const tailLimit = 60
	if len(runes) <= tailLimit {
		return content
	}
	return string(runes[len(runes)-tailLimit:])
}

func continuationChangeMetadata() workspacechange.ChangeMetadata {
	return workspacechange.ChangeMetadata{
		Origin:        "continuation",
		ChangeGroupID: fmt.Sprintf("continuation-%d", time.Now().UnixNano()),
		AutoAccept:    true,
	}
}
