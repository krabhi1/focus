# Focus

Focus is a productivity tool designed to keep your computer usage intentional. It helps you stay on track and manage your time effectively, ensuring every session at your desk has a clear purpose rather than drifting into aimless browsing.

The system operates via a background daemon (`focusd`) that handles time tracking, while a simple CLI (`focus`) provides the interface for managing your work.

## Rules

- **Intentional Use:** Focus gives you a 5-minute grace period to use your computer without an active task. Beyond that, the system will warn you before locking the screen or putting the computer to sleep.
- **One Thing at a Time:** Only one task can be active at once. When you finish a task, the daemon enforces a cooldown before you can start the next one. Shorter sessions get a shorter wait, longer sessions get a longer wait.
- **Work Modes:**
  - **Light Work (25 min):** For routine tasks that allow for multitasking. Requires a 5-minute break and concludes with a screen lock.
  - **Deep Work (90 min):** For high-concentration sessions without distractions. Requires a 15-minute break and concludes with the system sleeping.
- **Mandatory Breaks:** To prevent burnout, you cannot start a new task until your mandatory break is finished. You are welcome to take a longer break if needed, but the system ensures you get the minimum rest required.
- **Automatic Tracking:** There is no need for manual toggling. Focus monitors your keyboard, mouse, and screen activity to automatically pause and resume tasks whenever you step away.
- **Heads-up Notifications:** You’ll receive a notification 5 minutes before a task ends, giving you time to wrap up your work gracefully.
- **Deep Work Intermission:** To keep your mind fresh, deep work includes a short reset at the 45-minute mark. Focus will notify you and play a sound; one minute later, it locks the screen for 5 minutes so you can stretch and recharge.

## System Requirements

For now this is only available in Linux and tested with cinnamon desktop environment. It should work in other environments as well, but I haven't tested it yet.
