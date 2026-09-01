#include "timer.h"
#include "scheduler_internal.h"

#include <math.h>
#include <pthread.h>
#include <stdatomic.h>
#include <stdint.h>
#include <stdlib.h>
#include <time.h>

typedef struct tsnative_timer_waiter {
  struct tsnative_timer_waiter *next;
  tsnative_task *task;
  uint64_t deadline_ns;
  _Atomic int completed;
  int cooperative;
} tsnative_timer_waiter;

typedef struct {
  pthread_mutex_t mutex;
  pthread_cond_t changed;
  pthread_t thread;
  tsnative_timer_waiter *head;
  int started;
  int stopping;
} tsnative_timer_state;

static tsnative_timer_state timer_state = {
    .mutex = PTHREAD_MUTEX_INITIALIZER,
    .changed = PTHREAD_COND_INITIALIZER,
};

static uint64_t now_ns(void) {
  struct timespec ts;
  clock_gettime(CLOCK_MONOTONIC, &ts);
  return (uint64_t)ts.tv_sec * 1000000000ull + (uint64_t)ts.tv_nsec;
}

static uint64_t duration_ns(double milliseconds) {
  if (!isfinite(milliseconds) || milliseconds < 0) abort();
  double ns = milliseconds * 1000000.0;
  if (ns > (double)UINT64_MAX) abort();
  return (uint64_t)ns;
}

static struct timespec ns_to_timespec(uint64_t value) {
  struct timespec ts;
  ts.tv_sec = (time_t)(value / 1000000000ull);
  ts.tv_nsec = (long)(value % 1000000000ull);
  return ts;
}

static void insert_waiter_locked(tsnative_timer_waiter *waiter) {
  tsnative_timer_waiter **link = &timer_state.head;
  while (*link && (*link)->deadline_ns <= waiter->deadline_ns) link = &(*link)->next;
  waiter->next = *link;
  *link = waiter;
}

static void complete_waiter(tsnative_timer_waiter *waiter) {
  if (waiter->cooperative) {
    atomic_store_explicit(&waiter->completed, 1, memory_order_release);
    pthread_mutex_lock(&timer_state.mutex);
    pthread_cond_broadcast(&timer_state.changed);
    pthread_mutex_unlock(&timer_state.mutex);
    return;
  }
  (void)tsnative_scheduler_wake(waiter->task);
  free(waiter);
}

static void *timer_main(void *unused) {
  (void)unused;
  pthread_mutex_lock(&timer_state.mutex);
  for (;;) {
    if (timer_state.stopping) break;
    if (!timer_state.head) {
      pthread_cond_wait(&timer_state.changed, &timer_state.mutex);
      continue;
    }
    uint64_t now = now_ns();
    if (timer_state.head->deadline_ns > now) {
      struct timespec wake = ns_to_timespec(timer_state.head->deadline_ns);
      pthread_cond_timedwait(&timer_state.changed, &timer_state.mutex, &wake);
      continue;
    }
    tsnative_timer_waiter *waiter = timer_state.head;
    timer_state.head = waiter->next;
    pthread_mutex_unlock(&timer_state.mutex);
    complete_waiter(waiter);
    pthread_mutex_lock(&timer_state.mutex);
  }
  pthread_mutex_unlock(&timer_state.mutex);
  return NULL;
}

static int ensure_timer_started(void) {
  pthread_mutex_lock(&timer_state.mutex);
  if (timer_state.started) {
    pthread_mutex_unlock(&timer_state.mutex);
    return 0;
  }
  pthread_condattr_t attr;
  pthread_condattr_init(&attr);
  pthread_condattr_setclock(&attr, CLOCK_MONOTONIC);
  pthread_cond_destroy(&timer_state.changed);
  pthread_cond_init(&timer_state.changed, &attr);
  pthread_condattr_destroy(&attr);
  timer_state.stopping = 0;
  timer_state.head = NULL;
  timer_state.started = 1;
  int err = pthread_create(&timer_state.thread, NULL, timer_main, NULL);
  if (err != 0) timer_state.started = 0;
  pthread_mutex_unlock(&timer_state.mutex);
  return err == 0 ? 0 : -1;
}

int tsnative_sleep_task(double milliseconds) {
  if (duration_ns(milliseconds) == 0) return 1;
  if (ensure_timer_started() != 0) return -1;
  tsnative_task *task = tsnative_scheduler_current_task();
  if (!task || tsnative_scheduler_prepare_park() != 0) return -1;
  tsnative_timer_waiter *waiter = calloc(1, sizeof(*waiter));
  if (!waiter) {
    tsnative_scheduler_cancel_park();
    return -1;
  }
  waiter->task = task;
  waiter->deadline_ns = now_ns() + duration_ns(milliseconds);
  pthread_mutex_lock(&timer_state.mutex);
  insert_waiter_locked(waiter);
  pthread_cond_signal(&timer_state.changed);
  pthread_mutex_unlock(&timer_state.mutex);
  return 0;
}

static void sleep_blocking(double milliseconds) {
  uint64_t remaining = duration_ns(milliseconds);
  struct timespec req = ns_to_timespec(remaining);
  while (nanosleep(&req, &req) != 0) {}
}

void tsnative_sleep_cooperative(double milliseconds) {
  if (!tsnative_scheduler_current_task()) {
    sleep_blocking(milliseconds);
    return;
  }
  if (duration_ns(milliseconds) == 0) return;
  if (ensure_timer_started() != 0) abort();
  tsnative_timer_waiter *waiter = calloc(1, sizeof(*waiter));
  if (!waiter) abort();
  waiter->cooperative = 1;
  waiter->deadline_ns = now_ns() + duration_ns(milliseconds);
  pthread_mutex_lock(&timer_state.mutex);
  insert_waiter_locked(waiter);
  pthread_cond_signal(&timer_state.changed);
  pthread_mutex_unlock(&timer_state.mutex);
  while (!atomic_load_explicit(&waiter->completed, memory_order_acquire)) {
    if (tsnative_scheduler_help_once()) continue;
    pthread_mutex_lock(&timer_state.mutex);
    if (!atomic_load_explicit(&waiter->completed, memory_order_acquire)) {
      pthread_cond_wait(&timer_state.changed, &timer_state.mutex);
    }
    pthread_mutex_unlock(&timer_state.mutex);
  }
  free(waiter);
}

void tsnative_timer_shutdown(void) {
  pthread_mutex_lock(&timer_state.mutex);
  if (!timer_state.started) {
    pthread_mutex_unlock(&timer_state.mutex);
    return;
  }
  timer_state.stopping = 1;
  pthread_cond_broadcast(&timer_state.changed);
  pthread_mutex_unlock(&timer_state.mutex);
  pthread_join(timer_state.thread, NULL);

  pthread_mutex_lock(&timer_state.mutex);
  tsnative_timer_waiter *waiter = timer_state.head;
  timer_state.head = NULL;
  timer_state.started = 0;
  timer_state.stopping = 0;
  pthread_mutex_unlock(&timer_state.mutex);
  while (waiter) {
    tsnative_timer_waiter *next = waiter->next;
    complete_waiter(waiter);
    waiter = next;
  }
}
