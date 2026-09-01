#include "scheduler.h"

#include <errno.h>
#include <pthread.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>

#define TSNATIVE_MAX_WORKERS 256u

typedef struct {
  pthread_mutex_t mutex;
  pthread_cond_t wake;
  pthread_t *threads;
  size_t worker_count;
  int started;
  int stopping;
} tsnative_scheduler;

static tsnative_scheduler scheduler = {
    .mutex = PTHREAD_MUTEX_INITIALIZER,
    .wake = PTHREAD_COND_INITIALIZER,
};
static size_t configured_workers(void) {
  long cpu_count = sysconf(_SC_NPROCESSORS_ONLN);
  size_t workers = cpu_count > 0 ? (size_t)cpu_count : 1u;
  const char *raw = getenv("TSNATIVE_WORKERS");
  if (raw && raw[0]) {
    char *end = NULL;
    errno = 0;
    unsigned long parsed = strtoul(raw, &end, 10);
    if (errno == 0 && end && *end == '\0' && parsed > 0) {
      workers = (size_t)parsed;
    }
  }
  if (workers > TSNATIVE_MAX_WORKERS) workers = TSNATIVE_MAX_WORKERS;
  return workers ? workers : 1u;
}

static void *worker_main(void *unused) {
  (void)unused;
  pthread_mutex_lock(&scheduler.mutex);
  while (!scheduler.stopping) {
    pthread_cond_wait(&scheduler.wake, &scheduler.mutex);
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
  scheduler.threads = calloc(scheduler.worker_count, sizeof(*scheduler.threads));
  if (!scheduler.threads) {
    pthread_mutex_unlock(&scheduler.mutex);
    return -1;
  }
  scheduler.started = 1;
  scheduler.stopping = 0;
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
