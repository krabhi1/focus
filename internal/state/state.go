package state

import (
	"fmt"
	"go-basic/internal/sys"
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
	// Stop if there is an active task
	if state.CurrentTask != nil {
		state.StopCurrentTask()
	}
	task := Task{
		ID:        len(Global.TaskHistory) + 1,
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
		fmt.Printf("Task expired: %+v\n", state.CurrentTask)
		sys.Notify("Task expired", fmt.Sprintf("'%s' has expired. Screen is going to Lock", task.Title))
		time.Sleep(5 * time.Second) // Give user some time to see the notification before locking
		// sys.LockScreen()
		state.CurrentTask.Status = "completed"
		state.CurrentTask = nil
	})
	return task
}
func (state *DaemonState) StopCurrentTask() {
	if state.CurrentTask != nil {
		state.CurrentTask.Status = "stopped"
		sys.Notify("Task stopped", fmt.Sprintf("'%s' has been stopped.", state.CurrentTask.Title))
		state.CurrentTask = nil
		if state.BeforeExpireTimer != nil {
			state.BeforeExpireTimer.Stop()
		}
		if state.ExpireTimer != nil {
			state.ExpireTimer.Stop()
		}
	}
}

func (state *DaemonState) GetStatus() string {
	if state.CurrentTask == nil {
		return "No active task"
	}
	expireTime := state.CurrentTask.StartTime.Add(state.CurrentTask.Duration)
	return fmt.Sprintf("Current task: %s, expires in %s", state.CurrentTask.Title, expireTime)
}
