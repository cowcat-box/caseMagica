package continuation

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"casemagica/internal/workspacepath"
)

// Store 管理推演任务的落盘读写（.casemagica/continuations/<task-id>/）。
type Store struct {
	workspace string
}

// ContinuationsDir 是推演任务目录（相对 workspace）。
const ContinuationsDir = "continuations"

var (
	// ErrTaskNotFound 表示任务不存在。
	ErrTaskNotFound = errors.New("continuation task not found")
	// ErrTaskInProgress 表示同工作区已有进行中的推演任务。
	ErrTaskInProgress = errors.New("该工作区已有进行中的续写推演")
)

// NewStore 创建任务存储。
func NewStore(workspace string) *Store {
	return &Store{workspace: workspace}
}

// Root 返回任务根目录绝对路径。
func (s *Store) Root() string {
	return filepath.Join(workspacepath.Path(s.workspace, ContinuationsDir))
}

// TaskDir 返回单个任务目录。
func (s *Store) TaskDir(taskID string) string {
	return filepath.Join(s.Root(), taskID)
}

// SaveMeta 写入任务元信息。
func (s *Store) SaveMeta(meta TaskMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal task meta failed: %w", err)
	}
	return s.atomicWrite(filepath.Join(s.TaskDir(meta.ID), "meta.json"), data)
}

// LoadMeta 读取任务元信息。
func (s *Store) LoadMeta(taskID string) (TaskMeta, error) {
	data, err := os.ReadFile(filepath.Join(s.TaskDir(taskID), "meta.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return TaskMeta{}, ErrTaskNotFound
		}
		return TaskMeta{}, fmt.Errorf("read task meta failed: %w", err)
	}
	var meta TaskMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return TaskMeta{}, fmt.Errorf("parse task meta failed: %w", err)
	}
	return meta, nil
}

// ListMetas 列出工作区全部任务元信息（按创建时间倒序）。
func (s *Store) ListMetas() ([]TaskMeta, error) {
	entries, err := os.ReadDir(s.Root())
	if err != nil {
		if os.IsNotExist(err) {
			return []TaskMeta{}, nil
		}
		return nil, fmt.Errorf("read continuations dir failed: %w", err)
	}
	metas := []TaskMeta{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		meta, err := s.LoadMeta(entry.Name())
		if err != nil {
			continue
		}
		metas = append(metas, meta)
	}
	for i := 0; i < len(metas); i++ {
		for j := i + 1; j < len(metas); j++ {
			if metas[j].CreatedAt > metas[i].CreatedAt {
				metas[i], metas[j] = metas[j], metas[i]
			}
		}
	}
	return metas, nil
}

// SaveCandidate 写入单个候选。
func (s *Store) SaveCandidate(taskID string, candidate Candidate) error {
	data, err := json.MarshalIndent(candidate, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal candidate failed: %w", err)
	}
	return s.atomicWrite(filepath.Join(s.TaskDir(taskID), "candidates", fmt.Sprintf("%d.json", candidate.Index)), data)
}

// LoadCandidate 读取单个候选。
func (s *Store) LoadCandidate(taskID string, index int) (Candidate, error) {
	data, err := os.ReadFile(filepath.Join(s.TaskDir(taskID), "candidates", fmt.Sprintf("%d.json", index)))
	if err != nil {
		return Candidate{}, err
	}
	var candidate Candidate
	if err := json.Unmarshal(data, &candidate); err != nil {
		return Candidate{}, err
	}
	return candidate, nil
}

// LoadCandidates 读取任务全部候选（index 升序）。
func (s *Store) LoadCandidates(taskID string) ([]Candidate, error) {
	entries, err := os.ReadDir(filepath.Join(s.TaskDir(taskID), "candidates"))
	if err != nil {
		if os.IsNotExist(err) {
			return []Candidate{}, nil
		}
		return nil, err
	}
	candidates := []Candidate{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		index := parseCandidateIndex(entry.Name())
		if index < 0 {
			continue
		}
		candidate, err := s.LoadCandidate(taskID, index)
		if err != nil {
			continue
		}
		candidates = append(candidates, candidate)
	}
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].Index < candidates[i].Index {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}
	return candidates, nil
}

func parseCandidateIndex(filename string) int {
	base := strings.TrimSuffix(filename, ".json")
	index := 0
	digits := 0
	for _, ch := range base {
		if ch < '0' || ch > '9' {
			return -1
		}
		index = index*10 + int(ch-'0')
		digits++
	}
	if digits == 0 {
		return -1
	}
	return index
}

// RemoveTask 删除任务目录（调用方先备份）。
func (s *Store) RemoveTask(taskID string) error {
	return os.RemoveAll(s.TaskDir(taskID))
}

func (s *Store) atomicWrite(absPath string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return fmt.Errorf("create task dir failed: %w", err)
	}
	tmp := fmt.Sprintf("%s.tmp-%d", absPath, time.Now().UnixNano())
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write task file failed: %w", err)
	}
	if err := os.Rename(tmp, absPath); err != nil {
		return fmt.Errorf("commit task file failed: %w", err)
	}
	return nil
}
