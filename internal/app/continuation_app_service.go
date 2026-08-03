package app

import (
	"context"
	"fmt"
	"log"
	"sync"

	"casemagica/config"
	"casemagica/internal/book"
	"casemagica/internal/continuation"
	"casemagica/internal/workspacechange"
)

// ContinuationAppService 是续写推演的编排层：任务注册表（工作区级单实例）、
// 候选生成与提交写入。
type ContinuationAppService struct {
	app *App

	mu   sync.Mutex
	svcs map[string]*continuation.Service

	generatorFactory func(cfg *config.Config, service *book.Service, concurrency int) *continuation.Generator
}

func newContinuationAppService(a *App) *ContinuationAppService {
	return &ContinuationAppService{app: a, svcs: map[string]*continuation.Service{}}
}

func (s *ContinuationAppService) service(workspace string) *continuation.Service {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing := s.svcs[workspace]; existing != nil {
		return existing
	}
	service := continuation.NewService(workspace)
	s.svcs[workspace] = service
	return service
}

func (s *ContinuationAppService) configFor(workspace string) (*config.Config, error) {
	cfg := s.app.cfg
	if cfg == nil {
		return nil, fmt.Errorf("配置不存在")
	}
	resolved, _, err := config.LoadWithWorkspace(workspace)
	if err != nil {
		return nil, err
	}
	return resolved, nil
}

// ContinuationExplore 发起续写推演（App 层导出，供 API 调用）。
func (a *App) ContinuationExplore(request continuation.ExploreRequest) (*continuation.Task, error) {
	return a.continuation().Explore(request)
}

// ContinuationRunGeneration 执行候选生成（同步阻塞，调用方负责 goroutine 与 SSE 生命周期）。
func (a *App) ContinuationRunGeneration(task *continuation.Task, handler continuation.ExploreHandler) continuation.TaskStatus {
	return a.continuation().RunGeneration(task, handler)
}

// ContinuationTaskList 列出工作区推演任务。
func (a *App) ContinuationTaskList(workspace string) ([]continuation.TaskMeta, error) {
	return a.continuation().TaskList(workspace)
}

// ContinuationTask 按 ID 读取任务。
func (a *App) ContinuationTask(taskID string) (*continuation.Task, error) {
	return a.continuation().Task(taskID)
}

// ContinuationAbortTask 中止推演任务。
func (a *App) ContinuationAbortTask(taskID string) error {
	return a.continuation().AbortTask(taskID)
}

// ContinuationDeleteTask 删除推演任务（备份）。
func (a *App) ContinuationDeleteTask(taskID string) (string, error) {
	return a.continuation().DeleteTask(taskID)
}

// ContinuationCommit 提交候选。
func (a *App) ContinuationCommit(request continuation.CommitRequest) (continuation.CommitResult, error) {
	return a.continuation().Commit(request)
}

// Explore 校验请求并创建推演任务（同工作区进行中任务互斥）。
func (s *ContinuationAppService) Explore(request continuation.ExploreRequest) (*continuation.Task, error) {
	if _, err := validateBookWorkspacePath(request.Workspace); err != nil {
		return nil, err
	}
	depth, ok := continuation.NormalizeDepth(request.Depth)
	if !ok {
		return nil, fmt.Errorf("不支持的推演深度: %s", request.Depth)
	}
	request.Depth = string(depth)
	mode, ok := continuation.NormalizeMode(request.Mode, depth)
	if !ok {
		return nil, fmt.Errorf("短推演仅支持方向级（direction）")
	}
	request.Mode = string(mode)
	if request.AnchorChapter != "" {
		service := book.NewService(request.Workspace)
		if _, err := service.ReadFile(request.AnchorChapter); err != nil {
			return nil, fmt.Errorf("锚点章节不存在: %s", request.AnchorChapter)
		}
	}
	_, cancel := context.WithCancel(context.Background())
	task, err := s.service(request.Workspace).Create(request, cancel)
	if err != nil {
		cancel()
		return nil, err
	}
	return task, nil
}

// SetContinuationGeneratorFactoryForTest 替换候选生成器构造（仅测试使用）。
func (a *App) SetContinuationGeneratorFactoryForTest(factory func(cfg *config.Config, service *book.Service, concurrency int) *continuation.Generator) {
	service := a.continuation()
	service.mu.Lock()
	defer service.mu.Unlock()
	service.generatorFactory = factory
}

// RunGeneration 执行候选生成（同步阻塞直到全部候选收敛；调用方负责 goroutine 与 SSE 生命周期）。
func (s *ContinuationAppService) RunGeneration(task *continuation.Task, handler continuation.ExploreHandler) continuation.TaskStatus {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("[continuation] generation panic recovered task=%s err=%v", task.ID, recovered)
			task.SetStatus(continuation.TaskStatusFailed)
		}
	}()
	cfg, err := s.configFor(task.Workspace)
	if err != nil {
		log.Printf("[continuation] load config failed task=%s err=%v", task.ID, err)
		task.SetStatus(continuation.TaskStatusFailed)
		return continuation.TaskStatusFailed
	}
	bookService := book.NewService(task.Workspace)
	prefixChars := cfg.Continuation.PrefixCharsOrDefault()
	bounded, err := continuation.BuildBoundedContext(bookService, task.Request, prefixChars)
	if err != nil {
		log.Printf("[continuation] build context failed task=%s err=%v", task.ID, err)
		task.SetStatus(continuation.TaskStatusFailed)
		return continuation.TaskStatusFailed
	}
	s.mu.Lock()
	factory := s.generatorFactory
	s.mu.Unlock()
	generator := continuation.NewGenerator(cfg, bookService, 2)
	if factory != nil {
		generator = factory(cfg, bookService, 2)
	}
	mode := continuation.Mode(task.Request.Mode)
	return generator.Generate(context.Background(), task, bounded, mode, cfg.Continuation.ExcerptMaxCharsOrDefault(), handler)
}

// TaskList 列出工作区全部推演任务。
func (s *ContinuationAppService) TaskList(workspace string) ([]continuation.TaskMeta, error) {
	return s.service(workspace).Store().ListMetas()
}

// Task 按 ID 读取任务（内存优先，磁盘兜底）。
func (s *ContinuationAppService) Task(taskID string) (*continuation.Task, error) {
	return s.lookupTask(taskID)
}

// lookupTask 在已知工作区服务中查找任务。
func (s *ContinuationAppService) lookupTask(taskID string) (*continuation.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var lastErr error
	for _, service := range s.svcs {
		task, err := service.Get(taskID)
		if err == nil {
			return task, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = continuation.ErrTaskNotFound
	}
	return nil, lastErr
}

// AbortTask 中止推演任务。
func (s *ContinuationAppService) AbortTask(taskID string) error {
	task, err := s.lookupTask(taskID)
	if err != nil {
		return err
	}
	task.Cancel()
	return nil
}

// DeleteTask 删除推演任务记录（备份到 backups/continuations/）。
func (s *ContinuationAppService) DeleteTask(taskID string) (string, error) {
	task, err := s.lookupTask(taskID)
	if err != nil {
		return "", err
	}
	return s.service(task.Workspace).Delete(taskID)
}

// Commit 提交候选：新建章节或追加章末，走 workspacechange 原子写。
func (s *ContinuationAppService) Commit(request continuation.CommitRequest) (continuation.CommitResult, error) {
	task, err := s.lookupTask(request.TaskID)
	if err != nil {
		return continuation.CommitResult{}, err
	}
	if task.Status != continuation.TaskStatusDone && task.Status != continuation.TaskStatusPartiallyDone {
		return continuation.CommitResult{}, fmt.Errorf("任务尚未完成，无法提交")
	}
	changeService, err := workspacechange.ForWorkspace(task.Workspace)
	if err != nil {
		return continuation.CommitResult{}, err
	}
	committer := continuation.NewCommitter(book.NewService(task.Workspace), changeService)
	result, err := committer.Commit(request, task)
	if err != nil {
		return continuation.CommitResult{}, err
	}
	log.Printf("[continuation] commit done task=%s index=%d target=%s path=%s", request.TaskID, request.Index, request.Target, result.Path)
	return result, nil
}
