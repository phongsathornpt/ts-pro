#include "scheduler.h"
#include "scheduler_internal.h"
#include "task_internal.h"

#include <errno.h>
#include <pthread.h>
#include <stdint.h>
#include <stdlib.h>
#include <unistd.h>

#define TSNATIVE_MAX_WORKERS 256u
#define TSNATIVE_DEFAULT_MAX_TASKS 1000000u
#define TSNATIVE_HARD_MAX_TASKS 10000000u

typedef struct {
  pthread_mutex_t mutex;
  pthread_cond_t wake;
  pthread_t *threads;
  size_t worker_count;
  size_t max_tasks;
  tsnative_task *head;
  tsnative_task *tail;
  size_t active_tasks;
  size_t peak_active_tasks;
  size_t runnable_tasks;
  uint64_t next_task_id;
  uint64_t spawned_tasks;
  uint64_t completed_tasks;
  int started;
  int stopping;
} tsnative_scheduler;

static tsnative_scheduler scheduler = {
    .mutex = PTHREAD_MUTEX_INITIALIZER,
    .wake = PTHREAD_COND_INITIALIZER,
};

static size_t parse_limit(const char *name, size_t fallback, size_t hard_max) {
  const char *raw = getenv(name);
  if (!raw || !raw[0]) return fallback;
  char *end = NULL;
  errno = 0;
  unsigned long parsed = strtoul(raw, &end, 10);
  if (errno != 0 || !end || *end != '\0' || parsed == 0) return fallback;
  size_t value = (size_t)parsed;
  return value > hard_max ? hard_max : value;
}
static size_t configured_workers(void) {
  long cpu_count = sysconf(_SC_NPROCESSORS_ONLN);
  size_t fallback = cpu_count > 0 ? (size_t)cpu_count : 1u;
  return parse_limit("TSNATIVE_WORKERS", fallback, TSNATIVE_MAX_WORKERS);
}

static size_t configured_max_tasks(void) {
  return parse_limit("TSNATIVE_MAX_TASKS", TSNATIVE_DEFAULT_MAX_TASKS, TSNATIVE_HARD_MAX_TASKS);
}

static tsnative_task *pop_task(void) {
  tsnative_task *task = scheduler.head;
  if (!task) return NULL;
  scheduler.head = task->next;
  if (!scheduler.head) scheduler.tail = NULL;
  task->next = NULL;
  if (scheduler.runnable_tasks) scheduler.runnable_tasks--;
  return task;
}

static int terminal(tsnative_task_status status) {
  return status == TSNATIVE_TASK_DONE || status == TSNATIVE_TASK_CANCELLED || status == TSNATIVE_TASK_FAILED;
}
static void *worker_main(void *unused) {
  (void)unused;
  pthread_mutex_lock(&scheduler.mutex);
  for (;;) {
    while (!scheduler.head && !scheduler.stopping) {
      pthread_cond_wait(&scheduler.wake, &scheduler.mutex);
    }
    if (!scheduler.head && scheduler.stopping) break;
    tsnative_task *task = pop_task();
    task->status = TSNATIVE_TASK_RUNNING;
    pthread_mutex_unlock(&scheduler.mutex);

    task->entry(task->state);

    pthread_mutex_lock(&scheduler.mutex);
    task->status = TSNATIVE_TASK_DONE;
    if (scheduler.active_tasks) scheduler.active_tasks--;
    scheduler.completed_tasks++;
    pthread_cond_broadcast(&scheduler.wake);
  }
  pthread_mutex_unlock(&scheduler.mutex);
  return NULL;
}
int tsnative_scheduler_init(void) {
  pthread_mutex_lock(&scheduler.mutex);
  if (scheduler.started) {
    pthread_mutex_unlock(&scheduler.mutex);
    return 0;
  }
  scheduler.worker_count = configured_workers();
  scheduler.max_tasks = configured_max_tasks();
  scheduler.threads = calloc(scheduler.worker_count, sizeof(*scheduler.threads));
  if (!scheduler.threads) {
    pthread_mutex_unlock(&scheduler.mutex);
    return -1;
  }
  scheduler.started = 1;
  scheduler.stopping = 0;
  scheduler.head = scheduler.tail = NULL;
  scheduler.active_tasks = scheduler.peak_active_tasks = 0;
  scheduler.runnable_tasks = 0;
  scheduler.next_task_id = scheduler.spawned_tasks = scheduler.completed_tasks = 0;
  size_t created = 0;
  for (; created < scheduler.worker_count; created++) {
    if (pthread_create(&scheduler.threads[created], NULL, worker_main, NULL) != 0) break;
  }
  if (created == scheduler.worker_count) {
    pthread_mutex_unlock(&scheduler.mutex);
    return 0;
  }
  scheduler.stopping = 1;
  pthread_cond_broadcast(&scheduler.wake);
  pthread_mutex_unlock(&scheduler.mutex);
  for (size_t i = 0; i < created; i++) pthread_join(scheduler.threads[i], NULL);

  pthread_mutex_lock(&scheduler.mutex);
  free(scheduler.threads);
  scheduler.threads = NULL;
  scheduler.worker_count = 0;
  scheduler.started = 0;
  scheduler.stopping = 0;
  pthread_mutex_unlock(&scheduler.mutex);
  return -1;
}

int tsnative_scheduler_submit(tsnative_task *task) {
  if (!task) return -1;
  pthread_mutex_lock(&scheduler.mutex);
  if (!scheduler.started || scheduler.stopping || scheduler.active_tasks >= scheduler.max_tasks) {
    pthread_mutex_unlock(&scheduler.mutex);
    return -1;
  }
  task->id = ++scheduler.next_task_id;
  task->status = TSNATIVE_TASK_RUNNABLE;
  task->next = NULL;
  if (scheduler.tail) scheduler.tail->next = task;
  else scheduler.head = task;
  scheduler.tail = task;
  scheduler.active_tasks++;
  scheduler.runnable_tasks++;
  scheduler.spawned_tasks++;
  if (scheduler.active_tasks > scheduler.peak_active_tasks) {
    scheduler.peak_active_tasks = scheduler.active_tasks;
  }
  pthread_cond_signal(&scheduler.wake);
  pthread_mutex_unlock(&scheduler.mutex);
  return 0;
}

int tsnative_scheduler_wait(tsnative_task *task) {
  if (!task) return -1;
  pthread_mutex_lock(&scheduler.mutex);
  while (!terminal(task->status)) {
    pthread_cond_wait(&scheduler.wake, &scheduler.mutex);
  }
  int result = task->status == TSNATIVE_TASK_DONE ? 0 : -1;
  pthread_mutex_unlock(&scheduler.mutex);
  return result;
}
tsnative_task_status tsnative_scheduler_task_status(tsnative_task *task) {
  if (!task) return TSNATIVE_TASK_FAILED;
  pthread_mutex_lock(&scheduler.mutex);
  tsnative_task_status status = task->status;
  pthread_mutex_unlock(&scheduler.mutex);
  return status;
}

void tsnative_scheduler_shutdown(void) {
  pthread_mutex_lock(&scheduler.mutex);
  if (!scheduler.started) {
    pthread_mutex_unlock(&scheduler.mutex);
    return;
  }
  scheduler.stopping = 1;
  pthread_cond_broadcast(&scheduler.wake);
  pthread_t *threads = scheduler.threads;
  size_t count = scheduler.worker_count;
  pthread_mutex_unlock(&scheduler.mutex);

  for (size_t i = 0; i < count; i++) pthread_join(threads[i], NULL);

  pthread_mutex_lock(&scheduler.mutex);
  free(scheduler.threads);
  scheduler.threads = NULL;
  scheduler.worker_count = 0;
  scheduler.started = 0;
  scheduler.stopping = 0;
  scheduler.head = scheduler.tail = NULL;
  scheduler.runnable_tasks = 0;
  pthread_mutex_unlock(&scheduler.mutex);
}

size_t tsnative_scheduler_worker_count(void) {
  pthread_mutex_lock(&scheduler.mutex);
  size_t count = scheduler.started ? scheduler.worker_count : configured_workers();
  pthread_mutex_unlock(&scheduler.mutex);
  return count;
}

int tsnative_scheduler_is_running(void) {
  pthread_mutex_lock(&scheduler.mutex);
  int running = scheduler.started && !scheduler.stopping;
  pthread_mutex_unlock(&scheduler.mutex);
  return running;
}

size_t tsnative_scheduler_active_tasks(void) {
  pthread_mutex_lock(&scheduler.mutex);
  size_t value = scheduler.active_tasks;
  pthread_mutex_unlock(&scheduler.mutex);
  return value;
}
size_t tsnative_scheduler_peak_active_tasks(void) {
  pthread_mutex_lock(&scheduler.mutex);
  size_t value = scheduler.peak_active_tasks;
  pthread_mutex_unlock(&scheduler.mutex);
  return value;
}

uint64_t tsnative_scheduler_spawned_tasks(void) {
  pthread_mutex_lock(&scheduler.mutex);
  uint64_t value = scheduler.spawned_tasks;
  pthread_mutex_unlock(&scheduler.mutex);
  return value;
}

uint64_t tsnative_scheduler_completed_tasks(void) {
  pthread_mutex_lock(&scheduler.mutex);
  uint64_t value = scheduler.completed_tasks;
  pthread_mutex_unlock(&scheduler.mutex);
  return value;
}
