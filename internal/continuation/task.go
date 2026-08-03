package continuation

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"casemagica/internal/workspacepath"
)

// Service 管理推演任务生命周期：工作区级互斥、状态机与落盘。
type Service struct {
	store *Store

	mu          sync.Mutex
	byWorkspace map[string]*Task
	byID        map[string]*Task
}

// NewService 创建推演任务服务。
func NewService(workspace string) *Service {
	return &Service{
		store:       NewStore(workspace),
		byWorkspace: map[string]*Task{},
		byID:        map[string]*Task{},
	}
}

// Store 返回任务存储。
func (s *Service) Store() *Store {
	return s.store
}

// Task 是推演任务的运行期对象。
type Task struct {
	ID        string
	Workspace string
	Request   ExploreRequest
	Status    TaskStatus
	CreatedAt time.Time

	mu          sync.RWMutex
	candidates  map[int]*Candidate
	doneCount   int
	failedCount int
	cancel      context.CancelFunc
	persisted   bool
}

// Create 注册一个新任务（同工作区已有进行中任务时拒绝）。
func (s *Service) Create(request ExploreRequest, cancel context.CancelFunc) (*Task, error) {
	workspace := request.Workspace
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing := s.byWorkspace[workspace]; existing != nil && existing.IsActive() {
		return nil, ErrTaskInProgress
	}
	taskID := fmt.Sprintf("t%d", time.Now().UnixNano())
	task := &Task{
		ID:         taskID,
		Workspace:  workspace,
		Request:    request,
		Status:     TaskStatusPending,
		CreatedAt:  time.Now(),
		candidates: map[int]*Candidate{},
		cancel:     cancel,
	}
	s.byWorkspace[workspace] = task
	s.byID[taskID] = task

	meta := TaskMeta{
		ID:         taskID,
		Workspace:  workspace,
		Request:    request,
		Status:     string(TaskStatusPending),
		CreatedAt:  task.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  task.CreatedAt.Format(time.RFC3339),
	}
	if err := s.store.SaveMeta(meta); err != nil {
		log.Printf("[continuation] save task meta failed task=%s err=%v", taskID, err)
	}
	log.Printf("[continuation] task created task=%s workspace=%s depth=%s mode=%s anchor=%q", taskID, workspace, request.Depth, request.Mode, request.AnchorChapter)
	return task, nil
}

// Get 按 ID 查找任务（内存优先，磁盘兜底）。
func (s *Service) Get(taskID string) (*Task, error) {
	s.mu.Lock()
	task := s.byID[taskID]
	s.mu.Unlock()
	if task != nil {
		return task, nil
	}
	meta, err := s.store.LoadMeta(taskID)
	if err != nil {
		return nil, err
	}
	task = &Task{
		ID:         meta.ID,
		Workspace:  meta.Workspace,
		Request:    meta.Request,
		Status:     TaskStatus(meta.Status),
		CreatedAt:  parseTaskTime(meta.CreatedAt),
		candidates: map[int]*Candidate{},
		persisted:  true,
	}
	candidates, err := s.store.LoadCandidates(taskID)
	if err == nil {
		for _, candidate := range candidates {
			task.candidates[candidate.Index] = &candidate
			if candidate.Status == string(CandidateDone) {
				task.doneCount++
			} else if candidate.Status == string(CandidateFailed) {
				task.failedCount++
			}
		}
	}
	s.mu.Lock()
	if current := s.byID[taskID]; current != nil {
		task = current
	} else {
		s.byID[taskID] = task
	}
	s.mu.Unlock()
	return task, nil
}

// IsActive 返回任务是否仍在进行（进行中禁止同工作区新任务）。
func (t *Task) IsActive() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	switch t.Status {
	case TaskStatusPending, TaskStatusRunning:
		return true
	default:
		return false
	}
}

// StatusNow 返回当前状态与候选快照。
func (t *Task) StatusNow() (TaskStatus, []Candidate) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	candidates := make([]Candidate, 0, len(t.candidates))
	for _, candidate := range t.candidates {
		candidates = append(candidates, *candidate)
	}
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].Index < candidates[i].Index {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}
	return t.Status, candidates
}

// Candidate 返回单个候选副本。
func (t *Task) Candidate(index int) (Candidate, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	candidate, ok := t.candidates[index]
	if !ok {
		return Candidate{}, false
	}
	return *candidate, true
}

// UpdateCandidate 更新候选并落盘。
func (t *Task) UpdateCandidate(candidate Candidate) {
	t.mu.Lock()
	t.candidates[candidate.Index] = &candidate
	t.mu.Unlock()
	if err := t.persistCandidate(candidate); err != nil {
		log.Printf("[continuation] persist candidate failed task=%s index=%d err=%v", t.ID, candidate.Index, err)
	}
}

func (t *Task) persistCandidate(candidate Candidate) error {
	store := NewStore(t.Workspace)
	return store.SaveCandidate(t.ID, candidate)
}

// SetStatus 更新任务状态并落盘。
func (t *Task) SetStatus(status TaskStatus) {
	t.mu.Lock()
	t.Status = status
	t.mu.Unlock()
	meta := TaskMeta{
		ID:         t.ID,
		Workspace:  t.Workspace,
		Request:    t.Request,
		Status:     string(status),
		CreatedAt:  t.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  time.Now().Format(time.RFC3339),
	}
	if status == TaskStatusDone || status == TaskStatusPartiallyDone || status == TaskStatusCancelled || status == TaskStatusFailed {
		meta.FinishedAt = time.Now().Format(time.RFC3339)
	}
	if err := NewStore(t.Workspace).SaveMeta(meta); err != nil {
		log.Printf("[continuation] persist task status failed task=%s status=%s err=%v", t.ID, status, err)
	}
	log.Printf("[continuation] task status task=%s status=%s done=%d failed=%d", t.ID, status, t.doneCount, t.failedCount)
}

// Cancel 中止任务。
func (t *Task) Cancel() {
	t.mu.Lock()
	cancel := t.cancel
	active := t.Status == TaskStatusPending || t.Status == TaskStatusRunning
	if active {
		t.Status = TaskStatusCancelled
	}
	t.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if active {
		t.SetStatus(TaskStatusCancelled)
	}
}

// Complete 在生成结束后根据候选成败统计收敛状态。
func (t *Task) Complete() {
	t.mu.RLock()
	total := len(t.candidates)
	done := 0
	failed := 0
	for _, candidate := range t.candidates {
		switch candidate.Status {
		case string(CandidateDone):
			done++
		case string(CandidateFailed):
			failed++
		}
	}
	t.mu.RUnlock()
	switch {
	case total > 0 && failed > 0 && done > 0:
		t.SetStatus(TaskStatusPartiallyDone)
	case total > 0 && failed == total:
		t.SetStatus(TaskStatusFailed)
	case done > 0:
		t.SetStatus(TaskStatusDone)
	default:
		t.SetStatus(TaskStatusFailed)
	}
}

// Abort 通过注册表中止任务。
func (s *Service) Abort(taskID string) error {
	task, err := s.Get(taskID)
	if err != nil {
		return err
	}
	task.Cancel()
	return nil
}

// Delete 删除任务记录（先备份到 backups/continuations/）。
func (s *Service) Delete(taskID string) (string, error) {
	task, err := s.Get(taskID)
	if err != nil {
		return "", err
	}
	backupRoot := workspacepath.Path(task.Workspace, "backups") + "/continuations"
	backupPath := fmt.Sprintf("%s/%s-%d", backupRoot, taskID, time.Now().Unix())
	taskDir := s.store.TaskDir(taskID)
	if err := moveDirToBackup(taskDir, backupPath); err != nil {
		return "", err
	}
	s.mu.Lock()
	if current := s.byID[taskID]; current != nil {
		delete(s.byID, taskID)
		if s.byWorkspace[current.Workspace] == current {
			delete(s.byWorkspace, current.Workspace)
		}
	}
	s.mu.Unlock()
	log.Printf("[continuation] task deleted task=%s backup=%s", taskID, backupPath)
	return backupPath, nil
}

func moveDirToBackup(source, backupPath string) error {
	if _, err := os.Stat(source); err != nil {
		return ErrTaskNotFound
	}
	if err := os.MkdirAll(filepath.Dir(backupPath), 0o755); err != nil {
		return err
	}
	return os.Rename(source, backupPath)
}

func parseTaskTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}
