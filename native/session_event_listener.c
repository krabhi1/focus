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

#define FORMAT_TEXT 0
#define FORMAT_BINARY 1

#define EVENT_KIND_LISTENER 1
#define EVENT_KIND_SCREEN 2
#define EVENT_KIND_SLEEP 3
#define EVENT_KIND_SHUTDOWN 4

#define EVENT_STATE_NONE 0
#define EVENT_STATE_READY 1
#define EVENT_STATE_LOCKED 2
#define EVENT_STATE_UNLOCKED 3
#define EVENT_STATE_PREPARE 4
#define EVENT_STATE_RESUME 5
#define EVENT_STATE_CANCELLED 6

typedef struct {
    sd_bus *system_bus;
    sd_bus *session_bus;
    int output_format;
} AppState;

static void write_u16_le(uint8_t *dst, uint16_t value) {
    dst[0] = (uint8_t)(value & 0xffu);
    dst[1] = (uint8_t)((value >> 8) & 0xffu);
}

static void write_u64_le(uint8_t *dst, uint64_t value) {
    dst[0] = (uint8_t)(value & 0xffu);
    dst[1] = (uint8_t)((value >> 8) & 0xffu);
    dst[2] = (uint8_t)((value >> 16) & 0xffu);
    dst[3] = (uint8_t)((value >> 24) & 0xffu);
    dst[4] = (uint8_t)((value >> 32) & 0xffu);
    dst[5] = (uint8_t)((value >> 40) & 0xffu);
    dst[6] = (uint8_t)((value >> 48) & 0xffu);
    dst[7] = (uint8_t)((value >> 56) & 0xffu);
}

static uint64_t unix_milliseconds(void) {
    struct timespec ts;

    clock_gettime(CLOCK_REALTIME, &ts);
    return (uint64_t)ts.tv_sec * 1000ULL + (uint64_t)ts.tv_nsec / 1000000ULL;
}

static void format_timestamp(char *dst, size_t size) {
    time_t now = time(NULL);
    struct tm tm_now;

    localtime_r(&now, &tm_now);
    strftime(dst, size, "%Y-%m-%d %H:%M:%S", &tm_now);
}

static void emit_binary_event(uint8_t event_kind, uint8_t event_state) {
    uint8_t frame[24];
    size_t total_written = 0;

    memset(frame, 0, sizeof(frame));
    frame[0] = 'F';
    frame[1] = 'E';
    frame[2] = 'V';
    frame[3] = 1;
    frame[4] = event_kind;
    frame[5] = event_state;
    write_u16_le(&frame[6], 24);
    write_u64_le(&frame[8], unix_milliseconds());

    while (total_written < sizeof(frame)) {
        ssize_t written = write(STDOUT_FILENO, frame + total_written,
                                sizeof(frame) - total_written);
        if (written < 0) {
            if (errno == EINTR) {
                continue;
            }
            fprintf(stderr, "failed to write binary event\n");
            return;
        }
        total_written += (size_t)written;
    }
}

static void emit_text_event(const char *event, const char *state) {
    char stamp[32];

    format_timestamp(stamp, sizeof(stamp));
    printf("ts=\"%s\" event=%s", stamp, event);
    if (state != NULL) {
        printf(" state=%s", state);
    }
    putchar('\n');
    fflush(stdout);
}

static void emit_event(int output_format, uint8_t event_kind, uint8_t event_state,
                       const char *event, const char *state) {
    if (output_format == FORMAT_BINARY) {
        emit_binary_event(event_kind, event_state);
        return;
    }

    emit_text_event(event, state);
}

static int on_sleep_signal(sd_bus_message *message, void *userdata,
                           sd_bus_error *ret_error) {
    AppState *app = userdata;
    int sleeping = 0;

    (void)ret_error;

    if (sd_bus_message_read(message, "b", &sleeping) < 0) {
        fprintf(stderr, "failed to read PrepareForSleep payload\n");
        return 0;
    }

    emit_event(app->output_format, EVENT_KIND_SLEEP,
               sleeping ? EVENT_STATE_PREPARE : EVENT_STATE_RESUME, "sleep",
               sleeping ? "prepare" : "resume");
    return 0;
}

static int on_shutdown_signal(sd_bus_message *message, void *userdata,
                              sd_bus_error *ret_error) {
    AppState *app = userdata;
    int shutting_down = 0;

    (void)ret_error;

    if (sd_bus_message_read(message, "b", &shutting_down) < 0) {
        fprintf(stderr, "failed to read PrepareForShutdown payload\n");
        return 0;
    }

    emit_event(app->output_format, EVENT_KIND_SHUTDOWN,
               shutting_down ? EVENT_STATE_PREPARE : EVENT_STATE_CANCELLED,
               "shutdown", shutting_down ? "prepare" : "cancelled");
    return 0;
}

static int on_lock_signal(sd_bus_message *message, void *userdata,
                          sd_bus_error *ret_error) {
    AppState *app = userdata;
    int active = 0;

    (void)ret_error;

    if (sd_bus_message_read(message, "b", &active) < 0) {
        fprintf(stderr, "failed to read ActiveChanged payload\n");
        return 0;
    }

    emit_event(app->output_format, EVENT_KIND_SCREEN,
               active ? EVENT_STATE_LOCKED : EVENT_STATE_UNLOCKED, "screen",
               active ? "locked" : "unlocked");
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
        fprintf(stderr, "failed to subscribe to ActiveChanged: %s\n",
                strerror(-r));
        return r;
    }

    return 0;
}

static void usage(const char *prog) {
    fprintf(stderr, "Usage: %s [--format=text|binary]\n", prog);
    fprintf(stderr, "Example: %s\n", prog);
    fprintf(stderr, "Example: %s --format=binary\n", prog);
}

static int parse_format(const char *value) {
    if (strcmp(value, "text") == 0) {
        return FORMAT_TEXT;
    }
    if (strcmp(value, "binary") == 0) {
        return FORMAT_BINARY;
    }
    return -1;
}

int main(int argc, char *argv[]) {
    AppState app = {0};
    int argi = 1;
    int r;

    app.output_format = FORMAT_TEXT;

    if (argc > 1 && strncmp(argv[1], "--format=", 9) == 0) {
        app.output_format = parse_format(argv[1] + 9);
        if (app.output_format < 0) {
            usage(argv[0]);
            return 1;
        }
        argi++;
    } else if (argc > 2 && strcmp(argv[1], "--format") == 0) {
        app.output_format = parse_format(argv[2]);
        if (app.output_format < 0) {
            usage(argv[0]);
            return 1;
        }
        argi += 2;
    }

    if (argc - argi != 0) {
        usage(argv[0]);
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
        return 1;
    }

    emit_event(app.output_format, EVENT_KIND_LISTENER, EVENT_STATE_READY,
               "listener", "ready");

    while (true) {
        struct pollfd fds[2];
        nfds_t nfds = 0;
        int timeout_ms = -1;

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
            fprintf(stderr, "failed to process session bus: %s\n",
                    strerror(-r));
            break;
        }
    }

    sd_bus_unref(app.system_bus);
    sd_bus_unref(app.session_bus);
    return 1;
}
