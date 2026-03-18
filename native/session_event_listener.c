#include <X11/Xlib.h>
#include <X11/extensions/scrnsaver.h>
#include <errno.h>
#include <poll.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <systemd/sd-bus.h>
#include <time.h>
#include <unistd.h>

typedef struct {
  sd_bus *system_bus;
  sd_bus *session_bus;
  Display *display;
  unsigned int idle_threshold_seconds;
  unsigned int idle_poll_seconds;
  bool is_idle;
} AppState;

static void print_event(const char *event, const char *state) {
  char stamp[32];
  time_t now = time(NULL);
  struct tm tm_now;

  localtime_r(&now, &tm_now);
  strftime(stamp, sizeof(stamp), "%Y-%m-%d %H:%M:%S", &tm_now);

  printf("ts=\"%s\" event=%s", stamp, event);
  if (state != NULL) {
    printf(" state=%s", state);
  }
  putchar('\n');
  fflush(stdout);
}

static uint64_t monotonic_milliseconds(void) {
  struct timespec ts;

  clock_gettime(CLOCK_MONOTONIC, &ts);
  return (uint64_t)ts.tv_sec * 1000ULL + (uint64_t)ts.tv_nsec / 1000000ULL;
}

static unsigned long get_idle_milliseconds(Display *display) {
  XScreenSaverInfo *info = XScreenSaverAllocInfo();
  unsigned long idle_ms;

  if (info == NULL) {
    fprintf(stderr, "failed to allocate XScreenSaverInfo\n");
    return 0;
  }

  if (!XScreenSaverQueryInfo(display, DefaultRootWindow(display), info)) {
    fprintf(stderr, "failed to query X11 idle information\n");
    XFree(info);
    return 0;
  }

  idle_ms = info->idle;
  XFree(info);
  return idle_ms;
}

static void poll_idle_state(AppState *app) {
  unsigned long idle_ms = get_idle_milliseconds(app->display);
  bool now_idle =
      idle_ms >= (unsigned long)app->idle_threshold_seconds * 1000UL;

  if (now_idle && !app->is_idle) {
    print_event("idle", "entered");
    app->is_idle = true;
  } else if (!now_idle && app->is_idle) {
    print_event("idle", "exited");
    app->is_idle = false;
  }
}

static int on_sleep_signal(sd_bus_message *message, void *userdata,
                           sd_bus_error *ret_error) {
  int sleeping = 0;

  (void)userdata;
  (void)ret_error;

  if (sd_bus_message_read(message, "b", &sleeping) < 0) {
    fprintf(stderr, "failed to read PrepareForSleep payload\n");
    return 0;
  }

  print_event("sleep", sleeping ? "prepare" : "resume");
  return 0;
}

static int on_shutdown_signal(sd_bus_message *message, void *userdata,
                              sd_bus_error *ret_error) {
  int shutting_down = 0;

  (void)userdata;
  (void)ret_error;

  if (sd_bus_message_read(message, "b", &shutting_down) < 0) {
    fprintf(stderr, "failed to read PrepareForShutdown payload\n");
    return 0;
  }

  print_event("shutdown", shutting_down ? "prepare" : "cancelled");
  return 0;
}

static int on_lock_signal(sd_bus_message *message, void *userdata,
                          sd_bus_error *ret_error) {
  int active = 0;

  (void)userdata;
  (void)ret_error;

  if (sd_bus_message_read(message, "b", &active) < 0) {
    fprintf(stderr, "failed to read ActiveChanged payload\n");
    return 0;
  }

  print_event("screen", active ? "locked" : "unlocked");
  return 0;
}

static int process_bus(sd_bus *bus) {
  int r;

  while ((r = sd_bus_process(bus, NULL)) > 0) {
  }

  return r;
}

static int connect_buses(AppState *app) {
  int r;

  r = sd_bus_open_system(&app->system_bus);
  if (r < 0) {
    fprintf(stderr, "failed to connect to system bus: %s\n", strerror(-r));
    return r;
  }

  r = sd_bus_open_user(&app->session_bus);
  if (r < 0) {
    fprintf(stderr, "failed to connect to session bus: %s\n", strerror(-r));
    return r;
  }

  r = sd_bus_add_match(
      app->system_bus, NULL,
      "type='signal',sender='org.freedesktop.login1',"
      "interface='org.freedesktop.login1.Manager',"
      "member='PrepareForSleep',path='/org/freedesktop/login1'",
      on_sleep_signal, app);
  if (r < 0) {
    fprintf(stderr, "failed to subscribe to PrepareForSleep: %s\n",
            strerror(-r));
    return r;
  }

  r = sd_bus_add_match(
      app->system_bus, NULL,
      "type='signal',sender='org.freedesktop.login1',"
      "interface='org.freedesktop.login1.Manager',"
      "member='PrepareForShutdown',path='/org/freedesktop/login1'",
      on_shutdown_signal, app);
  if (r < 0) {
    fprintf(stderr, "failed to subscribe to PrepareForShutdown: %s\n",
            strerror(-r));
    return r;
  }

  r = sd_bus_add_match(
      app->session_bus, NULL,
      "type='signal',sender='org.cinnamon.ScreenSaver',"
      "interface='org.cinnamon.ScreenSaver',member='ActiveChanged',"
      "path='/org/cinnamon/ScreenSaver'",
      on_lock_signal, app);
  if (r < 0) {
    fprintf(stderr, "failed to subscribe to ActiveChanged: %s\n", strerror(-r));
    return r;
  }

  return 0;
}

static bool connect_display(AppState *app) {
  app->display = XOpenDisplay(NULL);
  if (app->display == NULL) {
    fprintf(stderr,
            "failed to open X display. Check DISPLAY and Xauthority.\n");
    return false;
  }

  return true;
}

static void usage(const char *prog) {
  fprintf(stderr, "Usage: %s <idle-threshold-seconds> [idle-poll-seconds]\n",
          prog);
  fprintf(stderr, "Example: %s 300\n", prog);
  fprintf(stderr, "Example: %s 300 2\n", prog);
}

int main(int argc, char *argv[]) {
  AppState app = {0};
  uint64_t idle_interval_ms;
  uint64_t next_idle_poll_ms;
  int r;

  if (argc < 2 || argc > 3) {
    usage(argv[0]);
    return 1;
  }

  app.idle_threshold_seconds = (unsigned int)strtoul(argv[1], NULL, 10);
  app.idle_poll_seconds =
      (argc == 3) ? (unsigned int)strtoul(argv[2], NULL, 10) : 1U;

  if (app.idle_threshold_seconds == 0 || app.idle_poll_seconds == 0) {
    usage(argv[0]);
    return 1;
  }

  if (!connect_display(&app)) {
    return 1;
  }

  r = connect_buses(&app);
  if (r < 0) {
    if (app.system_bus != NULL) {
      sd_bus_unref(app.system_bus);
    }
    if (app.session_bus != NULL) {
      sd_bus_unref(app.session_bus);
    }
    XCloseDisplay(app.display);
    return 1;
  }

  printf("ts=\"");
  {
    char stamp[32];
    time_t now = time(NULL);
    struct tm tm_now;
    localtime_r(&now, &tm_now);
    strftime(stamp, sizeof(stamp), "%Y-%m-%d %H:%M:%S", &tm_now);
    printf("%s", stamp);
  }
  printf("\" event=listener state=ready idle_threshold=%u idle_poll=%u\n",
         app.idle_threshold_seconds, app.idle_poll_seconds);
  fflush(stdout);

  poll_idle_state(&app);

  idle_interval_ms = (uint64_t)app.idle_poll_seconds * 1000ULL;
  next_idle_poll_ms = monotonic_milliseconds() + idle_interval_ms;

  while (true) {
    struct pollfd fds[2];
    nfds_t nfds = 0;
    uint64_t now_ms = monotonic_milliseconds();
    int timeout_ms = 0;

    if (sd_bus_get_fd(app.system_bus) >= 0) {
      fds[nfds].fd = sd_bus_get_fd(app.system_bus);
      fds[nfds].events = POLLIN;
      fds[nfds].revents = 0;
      nfds++;
    }

    if (sd_bus_get_fd(app.session_bus) >= 0) {
      fds[nfds].fd = sd_bus_get_fd(app.session_bus);
      fds[nfds].events = POLLIN;
      fds[nfds].revents = 0;
      nfds++;
    }

    if (next_idle_poll_ms > now_ms) {
      timeout_ms = (int)(next_idle_poll_ms - now_ms);
    }

    r = poll(fds, nfds, timeout_ms);
    if (r < 0) {
      if (errno == EINTR) {
        continue;
      }
      fprintf(stderr, "poll failed: %s\n", strerror(errno));
      break;
    }

    r = process_bus(app.system_bus);
    if (r < 0) {
      fprintf(stderr, "failed to process system bus: %s\n", strerror(-r));
      break;
    }

    r = process_bus(app.session_bus);
    if (r < 0) {
      fprintf(stderr, "failed to process session bus: %s\n", strerror(-r));
      break;
    }

    now_ms = monotonic_milliseconds();
    if (now_ms >= next_idle_poll_ms) {
      poll_idle_state(&app);
      next_idle_poll_ms = now_ms + idle_interval_ms;
    }
  }

  sd_bus_unref(app.system_bus);
  sd_bus_unref(app.session_bus);
  XCloseDisplay(app.display);
  return 1;
}
