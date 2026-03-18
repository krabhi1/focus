# Focus

This is a productivity tool that helps you stay focused and manage your time effectively while using the computer.
Goal is you must know know what you are doing on computer and not just randomly browsing or using it without any purpose.
It use a daemon (focusd) and client cli (focus) architecture. The daemon runs in the background and manages the tasks and time tracking, while the client cli provides a user interface for interacting with the daemon.

### Rules

- You can not use computer more 5min without active Task. It will show a warning notification and lock screen or sleep after 5min of warning.
- Only 1 Task can be active at a time. Lock the screen or sleep after 5min of task completion.
- You can start a new task without taking a break. [Task,Break, Task, Break, ... ]
- Task can light or deep work. Light work is for tasks that require less focus and can be done while doing other things, while deep work is for tasks that require more focus and should be done without distractions. So after light lask lock screen and after deep task do sleep .
- Task will be paused/resume based on user activity like keyboard/mouse screen lock/unlock sleep/awake. So you don't have to worry about pausing or resuming tasks manually.
- Deep work is usually 90min long and light work is usually 25min long.
- light work need 5min break and deep work need 15min break. So after light task you can take 5min break and after deep task you can take 15min break. You can also take longer breaks if needed. This is just the constrain to start new task. You can take longer breaks if needed but you can't start new task until the break time is over.
- system will notify you 5min before the task is completed. So you can wrap up your work and prepare for the next task or break.
- on deep work in middle around 45min it will notify you to take a short break. So you can stretch, walk around, or do something else to refresh your mind before continuing with the task. logic is deep-work [45m(notify time to break, play sound, lock screen after 1min),lock(5min break)]
