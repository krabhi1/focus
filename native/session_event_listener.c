#include <X11/Xlib.h>
#include <X11/extensions/scrnsaver.h>
#include <gio/gio.h>
#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>

typedef struct {
  GMainLoop *loop;
  GDBusConnection *system_bus;
  GDBusConnection *session_bus;
  Display *display;
  guint sleep_sub_id;
  guint shutdown_sub_id;
  guint lock_sub_id;
  guint idle_timer_id;
  unsigned int idle_threshold_seconds;
  unsigned int idle_poll_seconds;
  bool is_idle;
} AppState;

static void print_event(const char *event, const char *state) {
  GDateTime *now = g_date_time_new_now_local();
  gchar *stamp = g_date_time_format(now, "%Y-%m-%d %H:%M:%S");

  printf("ts=\"%s\" event=%s", stamp, event);
  if (state != NULL) {
    printf(" state=%s", state);
  }
  putchar('\n');
  fflush(stdout);

  g_free(stamp);
  g_date_time_unref(now);
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

static gboolean poll_idle_state(gpointer user_data) {
  AppState *app = user_data;
  unsigned long idle_ms;
  bool now_idle;

  idle_ms = get_idle_milliseconds(app->display);
  now_idle = idle_ms >= (unsigned long)app->idle_threshold_seconds * 1000UL;

  if (now_idle && !app->is_idle) {
    print_event("idle", "entered");
    app->is_idle = true;
  } else if (!now_idle && app->is_idle) {
    print_event("idle", "exited");
    app->is_idle = false;
  }

  return G_SOURCE_CONTINUE;
}

static void on_sleep_signal(GDBusConnection *connection,
                            const gchar *sender_name, const gchar *object_path,
                            const gchar *interface_name,
                            const gchar *signal_name, GVariant *parameters,
                            gpointer user_data) {
  gboolean sleeping = FALSE;

  (void)connection;
  (void)sender_name;
  (void)object_path;
  (void)interface_name;
  (void)signal_name;
  (void)user_data;

  g_variant_get(parameters, "(b)", &sleeping);
  print_event("sleep", sleeping ? "prepare" : "resume");
}

static void on_shutdown_signal(GDBusConnection *connection,
                               const gchar *sender_name,
                               const gchar *object_path,
                               const gchar *interface_name,
                               const gchar *signal_name, GVariant *parameters,
                               gpointer user_data) {
  gboolean shutting_down = FALSE;

  (void)connection;
  (void)sender_name;
  (void)object_path;
  (void)interface_name;
  (void)signal_name;
  (void)user_data;

  g_variant_get(parameters, "(b)", &shutting_down);
  print_event("shutdown", shutting_down ? "prepare" : "cancelled");
}

static void on_lock_signal(GDBusConnection *connection,
                           const gchar *sender_name, const gchar *object_path,
                           const gchar *interface_name,
                           const gchar *signal_name, GVariant *parameters,
                           gpointer user_data) {
  gboolean active = FALSE;

  (void)connection;
  (void)sender_name;
  (void)object_path;
  (void)interface_name;
  (void)signal_name;
  (void)user_data;

  g_variant_get(parameters, "(b)", &active);
  print_event("screen", active ? "locked" : "unlocked");
}

static gboolean connect_buses(AppState *app, GError **error) {
  app->system_bus = g_bus_get_sync(G_BUS_TYPE_SYSTEM, NULL, error);
  if (app->system_bus == NULL) {
    return FALSE;
  }

  app->session_bus = g_bus_get_sync(G_BUS_TYPE_SESSION, NULL, error);
  if (app->session_bus == NULL) {
    return FALSE;
  }

  return TRUE;
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

static void subscribe_signals(AppState *app) {
  app->sleep_sub_id = g_dbus_connection_signal_subscribe(
      app->system_bus, "org.freedesktop.login1",
      "org.freedesktop.login1.Manager", "PrepareForSleep",
      "/org/freedesktop/login1", NULL, G_DBUS_SIGNAL_FLAGS_NONE,
      on_sleep_signal, NULL, NULL);

  app->shutdown_sub_id = g_dbus_connection_signal_subscribe(
      app->system_bus, "org.freedesktop.login1",
      "org.freedesktop.login1.Manager", "PrepareForShutdown",
      "/org/freedesktop/login1", NULL, G_DBUS_SIGNAL_FLAGS_NONE,
      on_shutdown_signal, NULL, NULL);

  app->lock_sub_id = g_dbus_connection_signal_subscribe(
      app->session_bus, "org.cinnamon.ScreenSaver", "org.cinnamon.ScreenSaver",
      "ActiveChanged", "/org/cinnamon/ScreenSaver", NULL,
      G_DBUS_SIGNAL_FLAGS_NONE, on_lock_signal, NULL, NULL);

  app->idle_timer_id =
      g_timeout_add_seconds(app->idle_poll_seconds, poll_idle_state, app);
}

static void unsubscribe_signals(AppState *app) {
  if (app->idle_timer_id != 0) {
    g_source_remove(app->idle_timer_id);
  }

  if (app->system_bus != NULL) {
    if (app->sleep_sub_id != 0) {
      g_dbus_connection_signal_unsubscribe(app->system_bus, app->sleep_sub_id);
    }
    if (app->shutdown_sub_id != 0) {
      g_dbus_connection_signal_unsubscribe(app->system_bus,
                                           app->shutdown_sub_id);
    }
  }

  if (app->session_bus != NULL && app->lock_sub_id != 0) {
    g_dbus_connection_signal_unsubscribe(app->session_bus, app->lock_sub_id);
  }
}

static void usage(const char *prog) {
  fprintf(stderr, "Usage: %s <idle-threshold-seconds> [idle-poll-seconds]\n",
          prog);
  fprintf(stderr, "Example: %s 300\n", prog);
  fprintf(stderr, "Example: %s 300 2\n", prog);
}

int main(int argc, char *argv[]) {
  AppState app = {0};
  GError *error = NULL;

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

  if (!connect_buses(&app, &error)) {
    fprintf(stderr, "failed to connect to D-Bus: %s\n", error->message);
    g_clear_error(&error);
    if (app.system_bus != NULL) {
      g_object_unref(app.system_bus);
    }
    if (app.session_bus != NULL) {
      g_object_unref(app.session_bus);
    }
    return 1;
  }

  if (!connect_display(&app)) {
    g_object_unref(app.system_bus);
    g_object_unref(app.session_bus);
    return 1;
  }

  subscribe_signals(&app);

  {
    GDateTime *now = g_date_time_new_now_local();
    gchar *stamp = g_date_time_format(now, "%Y-%m-%d %H:%M:%S");

    printf(
        "ts=\"%s\" event=listener state=ready idle_threshold=%u idle_poll=%u\n",
        stamp, app.idle_threshold_seconds, app.idle_poll_seconds);
    fflush(stdout);

    g_free(stamp);
    g_date_time_unref(now);
  }

  poll_idle_state(&app);

  app.loop = g_main_loop_new(NULL, FALSE);
  g_main_loop_run(app.loop);

  unsubscribe_signals(&app);
  g_main_loop_unref(app.loop);
  XCloseDisplay(app.display);
  g_object_unref(app.system_bus);
  g_object_unref(app.session_bus);
  return 0;
}
