package state

import (
	"fmt"
	"focus/internal/sys"
	"sync"
	"time"
)

type Task struct {
	ID        int
	Title     string
	Duration  time.Duration
	StartTime time.Time
	Status    string // "active", "completed", "stopped"
}
type DaemonState struct {
	mu                sync.Mutex
	CurrentTask       *Task
	TaskHistory       []Task
	BeforeExpireTimer *time.Timer
	ExpireTimer       *time.Timer
}

const SocketPath = "/tmp/focus.sock"

var Global = &DaemonState{
	CurrentTask: nil,
	TaskHistory: []Task{},
}

func (state *DaemonState) NewTask(title string, duration time.Duration) Task {
	state.mu.Lock()
	stoppedTitle, stopped := "", false

	if state.CurrentTask != nil {
		stoppedTitle, stopped = state.stopCurrentTaskLocked()
	}

	task := Task{
		ID:        len(state.TaskHistory) + 1,
		Title:     title,
		Duration:  duration,
		StartTime: time.Now(),
		Status:    "active",
	}
	expireTime := task.StartTime.Add(task.Duration)
	state.CurrentTask = &task
	state.TaskHistory = append(state.TaskHistory, *state.CurrentTask)
	//create a timer before 10 seconds of expire time to notify user
	beforeExpire := time.Until(expireTime.Add(-10 * time.Second))
	if beforeExpire > 0 {
		state.BeforeExpireTimer = time.AfterFunc(beforeExpire, func() {
			sys.Notify("Task expiring soon", fmt.Sprintf("'%s' will expire in 10 seconds", task.Title))
			time.Sleep(2 * time.Second)
			sys.PlaySound("assets/task-ending.mp3")
		})
	}
	state.ExpireTimer = time.AfterFunc(task.Duration, func() {
		state.completeCurrentTask(task.Title)
	})
	state.mu.Unlock()

	if stopped {
		sys.Notify("Task stopped", fmt.Sprintf("'%s' has been stopped.", stoppedTitle))
	}

	return task
}

func (state *DaemonState) StopCurrentTask() {
	state.mu.Lock()
	title, stopped := state.stopCurrentTaskLocked()
	state.mu.Unlock()

	if stopped {
		sys.Notify("Task stopped", fmt.Sprintf("'%s' has been stopped.", title))
	}
}

func (state *DaemonState) GetStatus() string {
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.CurrentTask == nil {
		return "No active task"
	}
	expireTime := state.CurrentTask.StartTime.Add(state.CurrentTask.Duration)
	return fmt.Sprintf("Current task: %s, expires in %s", state.CurrentTask.Title, expireTime)
}

func (state *DaemonState) CurrentTaskTitle() (string, bool) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.CurrentTask == nil {
		return "", false
	}

	return state.CurrentTask.Title, true
}

func (state *DaemonState) stopCurrentTaskLocked() (string, bool) {
	if state.CurrentTask == nil {
		return "", false
	}

	title := state.CurrentTask.Title
	state.CurrentTask.Status = "stopped"
	state.CurrentTask = nil
	if state.BeforeExpireTimer != nil {
		state.BeforeExpireTimer.Stop()
		state.BeforeExpireTimer = nil
	}
	if state.ExpireTimer != nil {
		state.ExpireTimer.Stop()
		state.ExpireTimer = nil
	}

	return title, true
}

func (state *DaemonState) completeCurrentTask(expectedTitle string) {
	state.mu.Lock()
	if state.CurrentTask == nil || state.CurrentTask.Title != expectedTitle {
		state.mu.Unlock()
		return
	}

	fmt.Printf("Task expired: %+v\n", state.CurrentTask)
	state.CurrentTask.Status = "completed"
	state.CurrentTask = nil
	state.ExpireTimer = nil
	state.BeforeExpireTimer = nil
	state.mu.Unlock()

	sys.Notify("Task expired", fmt.Sprintf("'%s' has expired. Screen is going to Lock", expectedTitle))
	time.Sleep(5 * time.Second)
	// sys.LockScreen()
}
