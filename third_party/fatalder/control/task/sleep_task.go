package task

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/Yeah114/Fatalder/defines"
	"github.com/Yeah114/Fatalder/control"
)

type SleepTask struct {
	control  control.Control
	id       int
	TaskType TaskType `json:"type"`
	Duration int      `json:"duration"`

	// 控制字段
	runned     bool
	ctx        context.Context
	cancel     context.CancelFunc
	parentCtx  context.Context
	mu         sync.Mutex
	isPaused   bool
	isRunning  bool
	remainTime time.Duration
	startTime  time.Time
}

func NewSleepTask() *SleepTask {
	return &SleepTask{
		TaskType:   TaskTypeSleep,
		remainTime: 0,
		isPaused:   false,
		isRunning:  false,
	}
}

func (s *SleepTask) SetControl(c defines.Control) {
	ctrl, ok := c.(*control.Control)
	if !ok {
		panic(ErrInvalidControl)
	}
	s.control = *ctrl
	s.id = s.control.GetTaskID()
}

func (s *SleepTask) ID() int {
	return s.id
}

func (s *SleepTask) Type() string {
	return string(s.TaskType)
}

func (s *SleepTask) FromJSON(str string) error {
	return json.Unmarshal([]byte(str), s)
}

func (s *SleepTask) ToJSON() (string, error) {
	bs, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return string(bs), nil
}

func (s *SleepTask) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	s.mu.Lock()
	if s.isRunning {
		s.mu.Unlock()
		return nil // 已经在运行
	}
	if s.remainTime == 0 {
		s.remainTime = time.Duration(s.Duration) * time.Second
	}
	s.parentCtx = ctx
	s.mu.Unlock()

	return s.executeTask(ctx)
}

func (s *SleepTask) Pause() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isRunning || s.isPaused {
		return nil // 没有在运行或已经暂停
	}

	// 计算剩余时间
	elapsed := time.Since(s.startTime)
	s.remainTime -= elapsed
	if s.remainTime < 0 {
		s.remainTime = 0
	}

	// 取消当前的睡眠
	if s.cancel != nil {
		s.cancel()
	}

	s.isPaused = true
	s.isRunning = false

	return nil
}

func (s *SleepTask) Resume() error {
	s.mu.Lock()

	if !s.isPaused || s.isRunning {
		s.mu.Unlock()
		return nil // 没有暂停或已经在运行
	}

	if s.remainTime <= 0 {
		s.mu.Unlock()
		return nil // 没有剩余时间
	}

	parent := s.parentCtx
	s.mu.Unlock()

	if parent == nil {
		parent = context.Background()
	}
	return s.executeTask(parent)
}

// executeTask 执行睡眠任务的公共逻辑
func (s *SleepTask) executeTask(parent context.Context) error {
	s.mu.Lock()

	// 设置运行状态
	s.isRunning = true
	s.isPaused = false

	// 创建上下文用于取消
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.startTime = time.Now()
	sleepDuration := s.remainTime

	s.mu.Unlock()

	// 同步执行睡眠，可以被中断
	timer := time.NewTimer(sleepDuration)
	defer timer.Stop()

	select {
	case <-timer.C:
		// 正常完成
		s.mu.Lock()
		s.remainTime = 0
		s.isRunning = false
		s.mu.Unlock()
		s.runned = true
	case <-s.ctx.Done():
		s.mu.Lock()
		paused := s.isPaused
		s.isRunning = false
		s.mu.Unlock()
		if paused {
			return nil
		}
		return context.Canceled
	case <-parent.Done():
		s.mu.Lock()
		s.isRunning = false
		s.isPaused = false
		s.mu.Unlock()
		return parent.Err()
	}

	return nil
}

func (s *SleepTask) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 取消当前的睡眠
	if s.cancel != nil {
		s.cancel()
	}

	// 重置状态
	s.isRunning = false
	s.isPaused = false
	s.remainTime = time.Duration(s.Duration) * time.Second // 重置为原始时间

	return nil
}

// IsRunning 返回任务是否正在运行
func (s *SleepTask) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isRunning
}

// IsPaused 返回任务是否已暂停
func (s *SleepTask) IsPaused() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isPaused
}

// GetRemainTime 返回剩余Sleep时间（秒）
func (s *SleepTask) GetRemainTime() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isRunning && !s.isPaused {
		// 如果正在运行，计算实时剩余时间
		elapsed := time.Since(s.startTime)
		remain := max(0, s.remainTime-elapsed)
		return remain.Seconds()
	}

	return s.remainTime.Seconds()
}

func (s *SleepTask) MakeInterruptJSON() (string, error) {
	data := map[string]any{
		"type": s.Type(),
		"remainTime": s.GetRemainTime(),
	}
	bs, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(bs), nil
}

func (s *SleepTask) RestoreInterruptJSON(str string) error {
	data := make(map[string]any)
	if err := json.Unmarshal([]byte(str), &data); err != nil {
		return err
	}
	val, existed := data["remainTime"]
	if !existed {
		return ErrInvalidInterruptData
	}
	if _, ok := val.(float64); !ok {
		return ErrInvalidInterruptData
	}
	s.remainTime = time.Duration(val.(float64)) * time.Second
	return nil
}
