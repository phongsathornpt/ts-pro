#include "blocking_pool.h"
#include "scheduler_internal.h"

#include <errno.h>
#include <pthread.h>
#include <stdatomic.h>
#include <stdlib.h>
#include <unistd.h>

#define TSNATIVE_DEFAULT_BLOCKING_WORKERS 4u
#define TSNATIVE_MAX_BLOCKING_WORKERS 256u
#define TSNATIVE_DEFAULT_MAX_BLOCKING_JOBS 1024u
#define TSNATIVE_HARD_MAX_BLOCKING_JOBS 1000000u

typedef struct tsnative_blocking_job {
  struct tsnative_blocking_job *next;
  tsnative_blocking_entry entry;
  void *state;
  pthread_mutex_t mutex;
  pthread_cond_t changed;
  tsnative_task *waiter;
  _Atomic int done;
} tsnative_blocking_job;

typedef struct {
  pthread_mutex_t mutex;
  pthread_cond_t wake;
  pthread_t *workers;
  tsnative_blocking_job *head;
  tsnative_blocking_job *tail;
  size_t worker_count;
} tsnative_blocking_pool;

static struct {
  tsnative_blocking_pool pool;
  size_t max_jobs;
  _Atomic size_t active_jobs;
  _Atomic size_t peak_active_jobs;
  _Atomic uint64_t submitted_jobs;
  _Atomic uint64_t completed_jobs;
  int started;
  int stopping;
} blocking = {
  .pool = {
    .mutex = PTHREAD_MUTEX_INITIALIZER,
    .wake = PTHREAD_COND_INITIALIZER,
  },
};

static size_t parse_limit(const char *name, size_t fallback, size_t hard_max) {
  const char *raw = getenv(name);
  if (!raw || !raw[0]) return fallback;
  char *end = NULL;
  errno = 0;
  unsigned long value = strtoul(raw, &end, 10);
  if (errno || !end || *end != '\0' || value == 0) return fallback;
  size_t parsed = (size_t)value;
  return parsed > hard_max ? hard_max : parsed;
}

static tsnative_blocking_job *pop_job_locked(void) {
  tsnative_blocking_job *job = blocking.pool.head;
  if (!job) return NULL;
  blocking.pool.head = job->next;
  if (!blocking.pool.head) blocking.pool.tail = NULL;
  job->next = NULL;
  return job;
}

static void complete_job(tsnative_blocking_job *job) {
  atomic_fetch_sub_explicit(&blocking.active_jobs, 1, memory_order_relaxed);
  atomic_fetch_add_explicit(&blocking.completed_jobs, 1, memory_order_relaxed);
  pthread_mutex_lock(&job->mutex);
  atomic_store_explicit(&job->done, 1, memory_order_release);
  tsnative_task *waiter = job->waiter;
  job->waiter = NULL;
  pthread_cond_broadcast(&job->changed);
  pthread_mutex_unlock(&job->mutex);
  if (waiter) (void)tsnative_scheduler_wake(waiter);
}

static void *blocking_worker(void *unused) {
  (void)unused;
  for (;;) {
    pthread_mutex_lock(&blocking.pool.mutex);
    while (!blocking.stopping && !blocking.pool.head) {
      pthread_cond_wait(&blocking.pool.wake, &blocking.pool.mutex);
    }
    if (blocking.stopping && !blocking.pool.head) {
      pthread_mutex_unlock(&blocking.pool.mutex);
      return NULL;
    }
    tsnative_blocking_job *job = pop_job_locked();
    pthread_mutex_unlock(&blocking.pool.mutex);
    if (!job) continue;
    job->entry(job->state);
    complete_job(job);
  }
}

static int ensure_blocking_pool_started(void) {
  pthread_mutex_lock(&blocking.pool.mutex);
  if (blocking.started) {
    pthread_mutex_unlock(&blocking.pool.mutex);
    return 0;
  }
  blocking.pool.worker_count = parse_limit("TSNATIVE_BLOCKING_WORKERS", TSNATIVE_DEFAULT_BLOCKING_WORKERS, TSNATIVE_MAX_BLOCKING_WORKERS);
  blocking.max_jobs = parse_limit("TSNATIVE_MAX_BLOCKING_JOBS", TSNATIVE_DEFAULT_MAX_BLOCKING_JOBS, TSNATIVE_HARD_MAX_BLOCKING_JOBS);
  blocking.pool.workers = calloc(blocking.pool.worker_count, sizeof(*blocking.pool.workers));
  if (!blocking.pool.workers) {
    pthread_mutex_unlock(&blocking.pool.mutex);
    return -1;
  }
  blocking.started = 1;
  blocking.stopping = 0;
  blocking.pool.head = blocking.pool.tail = NULL;
  atomic_store(&blocking.active_jobs, 0);
  atomic_store(&blocking.peak_active_jobs, 0);
  atomic_store(&blocking.submitted_jobs, 0);
  atomic_store(&blocking.completed_jobs, 0);
  size_t created = 0;
  for (; created < blocking.pool.worker_count; created++) {
    if (pthread_create(&blocking.pool.workers[created], NULL, blocking_worker, NULL) != 0) break;
  }
  if (created == blocking.pool.worker_count) {
    pthread_mutex_unlock(&blocking.pool.mutex);
    return 0;
  }
  blocking.stopping = 1;
  pthread_cond_broadcast(&blocking.pool.wake);
  pthread_mutex_unlock(&blocking.pool.mutex);
  for (size_t i = 0; i < created; i++) pthread_join(blocking.pool.workers[i], NULL);
  pthread_mutex_lock(&blocking.pool.mutex);
  free(blocking.pool.workers);
  blocking.pool.workers = NULL;
  blocking.pool.worker_count = 0;
  blocking.started = 0;
  blocking.stopping = 0;
  pthread_mutex_unlock(&blocking.pool.mutex);
  return -1;
}

tsnative_blocking_job *tsnative_blocking_submit(tsnative_blocking_entry entry, void *state) {
  if (!entry || ensure_blocking_pool_started() != 0) return NULL;
  pthread_mutex_lock(&blocking.pool.mutex);
  size_t active = atomic_load_explicit(&blocking.active_jobs, memory_order_relaxed);
  if (blocking.stopping || active >= blocking.max_jobs) {
    pthread_mutex_unlock(&blocking.pool.mutex);
    return NULL;
  }
  tsnative_blocking_job *job = calloc(1, sizeof(*job));
  if (!job) {
    pthread_mutex_unlock(&blocking.pool.mutex);
    return NULL;
  }
  job->entry = entry;
  job->state = state;
  pthread_mutex_init(&job->mutex, NULL);
  pthread_cond_init(&job->changed, NULL);
  if (blocking.pool.tail) blocking.pool.tail->next = job;
  else blocking.pool.head = job;
  blocking.pool.tail = job;
  active = atomic_fetch_add_explicit(&blocking.active_jobs, 1, memory_order_relaxed) + 1;
  atomic_fetch_add_explicit(&blocking.submitted_jobs, 1, memory_order_relaxed);
  size_t peak = atomic_load_explicit(&blocking.peak_active_jobs, memory_order_relaxed);
  if (active > peak) atomic_store_explicit(&blocking.peak_active_jobs, active, memory_order_relaxed);
  pthread_cond_signal(&blocking.pool.wake);
  pthread_mutex_unlock(&blocking.pool.mutex);
  return job;
}

int tsnative_blocking_job_wait_task(tsnative_blocking_job *job) {
  if (!job) return -1;
  if (atomic_load_explicit(&job->done, memory_order_acquire)) return 1;
  tsnative_task *task = tsnative_scheduler_current_task();
  if (!task || tsnative_scheduler_prepare_park() != 0) return -1;
  pthread_mutex_lock(&job->mutex);
  if (atomic_load_explicit(&job->done, memory_order_acquire)) {
    pthread_mutex_unlock(&job->mutex);
    tsnative_scheduler_cancel_park();
    return 1;
  }
  job->waiter = task;
  pthread_mutex_unlock(&job->mutex);
  return 0;
}

void tsnative_blocking_job_wait_cooperative(tsnative_blocking_job *job) {
  if (!job) abort();
  while (!atomic_load_explicit(&job->done, memory_order_acquire)) {
    if (tsnative_scheduler_help_once()) continue;
    pthread_mutex_lock(&job->mutex);
    if (!atomic_load_explicit(&job->done, memory_order_acquire)) {
      pthread_cond_wait(&job->changed, &job->mutex);
    }
    pthread_mutex_unlock(&job->mutex);
  }
}

void tsnative_blocking_job_release(tsnative_blocking_job *job) {
  if (!job) return;
  tsnative_blocking_job_wait_cooperative(job);
  pthread_cond_destroy(&job->changed);
  pthread_mutex_destroy(&job->mutex);
  free(job);
}

void tsnative_blocking_pool_shutdown(void) {
  pthread_mutex_lock(&blocking.pool.mutex);
  if (!blocking.started) {
    pthread_mutex_unlock(&blocking.pool.mutex);
    return;
  }
  blocking.stopping = 1;
  pthread_cond_broadcast(&blocking.pool.wake);
  pthread_t *workers = blocking.pool.workers;
  size_t count = blocking.pool.worker_count;
  pthread_mutex_unlock(&blocking.pool.mutex);
  for (size_t i = 0; i < count; i++) pthread_join(workers[i], NULL);
  pthread_mutex_lock(&blocking.pool.mutex);
  free(workers);
  blocking.pool.workers = NULL;
  blocking.pool.worker_count = 0;
  blocking.pool.head = blocking.pool.tail = NULL;
  blocking.started = 0;
  blocking.stopping = 0;
  pthread_mutex_unlock(&blocking.pool.mutex);
}

size_t tsnative_blocking_worker_count(void) {
  pthread_mutex_lock(&blocking.pool.mutex);
  size_t count = blocking.started ? blocking.pool.worker_count : parse_limit("TSNATIVE_BLOCKING_WORKERS", TSNATIVE_DEFAULT_BLOCKING_WORKERS, TSNATIVE_MAX_BLOCKING_WORKERS);
  pthread_mutex_unlock(&blocking.pool.mutex);
  return count;
}

size_t tsnative_blocking_active_jobs(void) { return atomic_load(&blocking.active_jobs); }
size_t tsnative_blocking_peak_active_jobs(void) { return atomic_load(&blocking.peak_active_jobs); }
uint64_t tsnative_blocking_submitted_jobs(void) { return atomic_load(&blocking.submitted_jobs); }
uint64_t tsnative_blocking_completed_jobs(void) { return atomic_load(&blocking.completed_jobs); }
