package continuation

// Depth 表示推演深度：短/中/长。
type Depth string

const (
	DepthShort  Depth = "short"
	DepthMedium Depth = "medium"
	DepthLong   Depth = "long"
)

// Mode 表示候选粒度：方向级或片段级。
type Mode string

const (
	ModeDirection Mode = "direction"
	ModeExcerpt   Mode = "excerpt"
)

// Target 表示提交落盘位置。
type Target string

const (
	TargetNewChapter Target = "new_chapter"
	TargetAppend     Target = "append"
)

// TaskStatus 是推演任务状态机（穷尽枚举，禁止默认分支吞掉新状态）。
type TaskStatus string

const (
	TaskStatusPending       TaskStatus = "pending"
	TaskStatusRunning       TaskStatus = "running"
	TaskStatusDone          TaskStatus = "done"
	TaskStatusPartiallyDone TaskStatus = "partially_done"
	TaskStatusCancelled     TaskStatus = "cancelled"
	TaskStatusFailed        TaskStatus = "failed"
)

// CandidateStatus 是单个候选的状态。
type CandidateStatus string

const (
	CandidateGenerating CandidateStatus = "generating"
	CandidateDone       CandidateStatus = "done"
	CandidateFailed     CandidateStatus = "failed"
)

// ExploreRequest 描述一次续写推演请求。
type ExploreRequest struct {
	Workspace     string   `json:"workspace"`
	AnchorChapter string   `json:"anchor_chapter_path,omitempty"` // 默认最近章节
	Depth         string   `json:"depth"`                         // short|medium|long
	Mode          string   `json:"mode"`                          // direction|excerpt
	StyleRefNames []string `json:"style_ref_names,omitempty"`     // .casemagica/styles 已有参考
	CustomStyle   string   `json:"custom_style,omitempty"`
	Preference    string   `json:"preference,omitempty"` // 方向偏好自由文本
}

// Candidate 是推演出的一个续写候选。
type Candidate struct {
	Index     int    `json:"index"`
	Title     string `json:"title,omitempty"`
	Direction string `json:"direction,omitempty"`
	Excerpt   string `json:"excerpt,omitempty"`
	Plan      string `json:"plan,omitempty"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
}

// TaskMeta 是任务落盘元信息（.casemagica/continuations/<task-id>/meta.json）。
type TaskMeta struct {
	ID           string        `json:"id"`
	Workspace    string        `json:"workspace"`
	Request      ExploreRequest `json:"request"`
	Status       string        `json:"status"`
	CreatedAt    string        `json:"created_at"`
	UpdatedAt    string        `json:"updated_at"`
	FinishedAt   string        `json:"finished_at,omitempty"`
}

// CommitRequest 描述提交一个候选。
type CommitRequest struct {
	TaskID  string `json:"task_id"`
	Index   int    `json:"index"`
	Title   string `json:"title"`
	Content string `json:"content"`
	Target  string `json:"target"` // new_chapter | append
}

// CommitResult 描述提交结果。
type CommitResult struct {
	Path string `json:"path"`
}

// NormalizeDepth 归一化深度并校验。
func NormalizeDepth(value string) (Depth, bool) {
	switch Depth(value) {
	case DepthShort, DepthMedium, DepthLong:
		return Depth(value), true
	default:
		return DepthMedium, false
	}
}

// NormalizeMode 归一化粒度并校验（短推演仅允许方向级）。
func NormalizeMode(value string, depth Depth) (Mode, bool) {
	switch Mode(value) {
	case ModeDirection:
		return ModeDirection, true
	case ModeExcerpt:
		if depth == DepthShort {
			return ModeExcerpt, false
		}
		return ModeExcerpt, true
	default:
		return ModeExcerpt, false
	}
}

// DefaultCandidateCount 返回深度对应的默认候选数（受配置上限约束）。
func DefaultCandidateCount(depth Depth, maxCandidates int) int {
	switch depth {
	case DepthShort:
		return minInt(2, maxCandidates)
	case DepthLong:
		return minInt(3, maxCandidates)
	default:
		return minInt(3, maxCandidates)
	}
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
