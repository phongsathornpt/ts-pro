#include "scheduler.h"
#include "scheduler_internal.h"
#include "task_internal.h"
#include "timer.h"
#include "blocking_pool.h"
#include "channel_f64.h"

#include <errno.h>
#include <pthread.h>
#include <stdatomic.h>
#include <stdint.h>
#include <stdlib.h>
#include <unistd.h>

#define TSNATIVE_MAX_WORKERS 256u
#define TSNATIVE_DEFAULT_MAX_TASKS 1000000u
#define TSNATIVE_HARD_MAX_TASKS 10000000u

typedef struct tsnative_worker {
  pthread_t thread;
  pthread_mutex_t deque_mutex;
  tsnative_task *head;
  tsnative_task *tail;
  size_t index;
} tsnative_worker;

typedef struct {
  pthread_mutex_t mutex;
  pthread_cond_t wake;
  pthread_cond_t task_done;
  tsnative_worker *workers;
  size_t worker_count;
  size_t max_tasks;
  tsnative_task *head;
  tsnative_task *tail;
  _Atomic size_t active_tasks;
  _Atomic size_t peak_active_tasks;
  _Atomic size_t runnable_tasks;
  uint64_t next_task_id;
  _Atomic uint64_t spawned_tasks;
  _Atomic uint64_t completed_tasks;
  _Atomic uint64_t steal_attempts;
  _Atomic uint64_t successful_steals;
  _Atomic uint64_t worker_parks;
  _Atomic uint64_t worker_wakeups;
  int started;
  int stopping;
} tsnative_scheduler;

static tsnative_scheduler scheduler = {
    .mutex = PTHREAD_MUTEX_INITIALIZER,
    .wake = PTHREAD_COND_INITIALIZER,
    .task_done = PTHREAD_COND_INITIALIZER,
};
static _Thread_local tsnative_worker *current_worker;
static _Thread_local tsnative_task *current_task;

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

static int terminal(int status) {
  return status == TSNATIVE_TASK_DONE || status == TSNATIVE_TASK_CANCELLED || status == TSNATIVE_TASK_FAILED;
}

static void worker_push_tail(tsnative_worker *worker, tsnative_task *task) {
  pthread_mutex_lock(&worker->deque_mutex);
  task->next = NULL;
  task->prev = worker->tail;
  if (worker->tail) worker->tail->next = task;
  else worker->head = task;
  worker->tail = task;
  pthread_mutex_unlock(&worker->deque_mutex);
}

static tsnative_task *worker_pop_tail(tsnative_worker *worker) {
  pthread_mutex_lock(&worker->deque_mutex);
  tsnative_task *task = worker->tail;
  if (task) {
    worker->tail = task->prev;
    if (worker->tail) worker->tail->next = NULL;
    else worker->head = NULL;
    task->next = task->prev = NULL;
  }
  pthread_mutex_unlock(&worker->deque_mutex);
  return task;
}

static tsnative_task *worker_steal_head(tsnative_worker *worker) {
  pthread_mutex_lock(&worker->deque_mutex);
  tsnative_task *task = worker->head;
  if (task) {
    worker->head = task->next;
    if (worker->head) worker->head->prev = NULL;
    else worker->tail = NULL;
    task->next = task->prev = NULL;
  }
  pthread_mutex_unlock(&worker->deque_mutex);
  return task;
}

static void injection_push_locked(tsnative_task *task) {
  task->next = NULL;
  task->prev = scheduler.tail;
  if (scheduler.tail) scheduler.tail->next = task;
  else scheduler.head = task;
  scheduler.tail = task;
}

static tsnative_task *injection_pop_locked(void) {
  tsnative_task *task = scheduler.head;
  if (!task) return NULL;
  scheduler.head = task->next;
  if (scheduler.head) scheduler.head->prev = NULL;
  else scheduler.tail = NULL;
  task->next = task->prev = NULL;
  return task;
}

static tsnative_task *steal_task(tsnative_worker *self) {
  size_t count = scheduler.worker_count;
  if (count < 2) return NULL;
  for (size_t offset = 1; offset < count; offset++) {
    size_t index = (self->index + offset) % count;
    atomic_fetch_add_explicit(&scheduler.steal_attempts, 1, memory_order_relaxed);
    tsnative_task *task = worker_steal_head(&scheduler.workers[index]);
    if (task) {
      atomic_fetch_add_explicit(&scheduler.successful_steals, 1, memory_order_relaxed);
      return task;
    }
  }
  return NULL;
}

static tsnative_task *take_work(tsnative_worker *worker) {
  tsnative_task *task = worker_pop_tail(worker);
  if (!task) {
    pthread_mutex_lock(&scheduler.mutex);
    task = injection_pop_locked();
    pthread_mutex_unlock(&scheduler.mutex);
  }
  if (!task) task = steal_task(worker);
  if (task) atomic_fetch_sub_explicit(&scheduler.runnable_tasks, 1, memory_order_relaxed);
  return task;
}

static void execute_task(tsnative_task *task) {
  atomic_store_explicit(&task->status, TSNATIVE_TASK_RUNNING, memory_order_release);
  atomic_store_explicit(&task->park_requested, 0, memory_order_release);
  atomic_store_explicit(&task->wake_requested, 0, memory_order_release);
  tsnative_task *previous_task = current_task;
  current_task = task;
  if (!atomic_load_explicit(&task->failure_requested, memory_order_acquire)) {
    task->entry(task->state, &task->result);
  }
  current_task = previous_task;

  pthread_mutex_lock(&scheduler.mutex);
  if (atomic_load_explicit(&task->park_requested, memory_order_acquire)) {
    if (atomic_exchange_explicit(&task->wake_requested, 0, memory_order_acq_rel)) {
      atomic_store_explicit(&task->status, TSNATIVE_TASK_RUNNABLE, memory_order_release);
      injection_push_locked(task);
      atomic_fetch_add_explicit(&scheduler.runnable_tasks, 1, memory_order_relaxed);
      pthread_cond_signal(&scheduler.wake);
    } else {
      atomic_store_explicit(&task->status, TSNATIVE_TASK_WAITING, memory_order_release);
    }
    pthread_cond_broadcast(&scheduler.task_done);
    pthread_mutex_unlock(&scheduler.mutex);
    return;
  }
  pthread_mutex_lock(&task->completion_mutex);
  int failed = atomic_load_explicit(&task->failure_requested, memory_order_acquire);
  atomic_store_explicit(&task->status, failed ? TSNATIVE_TASK_FAILED : TSNATIVE_TASK_DONE, memory_order_release);
  atomic_fetch_sub_explicit(&scheduler.active_tasks, 1, memory_order_relaxed);
  atomic_fetch_add_explicit(&scheduler.completed_tasks, 1, memory_order_relaxed);
  tsnative_task *completion_waiter = task->completion_waiter;
  void *completion_out = task->completion_out;
  int completion_consume = task->completion_consume;
  task->completion_waiter = NULL;
  task->completion_out = NULL;
  task->completion_consume = 0;
  if (completion_waiter && failed) {
    completion_waiter->failure_ref = task->failure_ref;
    atomic_store_explicit(&completion_waiter->failure_requested, 1, memory_order_release);
  } else if (completion_waiter && completion_out && task->transfer_completion) {
    task->transfer_completion(task, completion_out);
  }
  pthread_cond_broadcast(&scheduler.task_done);
  pthread_mutex_unlock(&scheduler.mutex);
  pthread_mutex_unlock(&task->completion_mutex);
  if (task->notify_completed) task->notify_completed(task);
  if (completion_waiter) (void)tsnative_scheduler_wake(completion_waiter);
  if (completion_consume && task->destroy_completed) task->destroy_completed(task);
}

static void *worker_main(void *arg) {
  tsnative_worker *worker = arg;
  current_worker = worker;
  for (;;) {
    tsnative_task *task = take_work(worker);
    if (!task) {
      pthread_mutex_lock(&scheduler.mutex);
      if (scheduler.stopping && atomic_load_explicit(&scheduler.runnable_tasks, memory_order_relaxed) == 0) {
        pthread_mutex_unlock(&scheduler.mutex);
        break;
      }
      while (!scheduler.stopping && scheduler.head == NULL &&
             atomic_load_explicit(&scheduler.runnable_tasks, memory_order_relaxed) == 0) {
        atomic_fetch_add_explicit(&scheduler.worker_parks, 1, memory_order_relaxed);
        pthread_cond_wait(&scheduler.wake, &scheduler.mutex);
        atomic_fetch_add_explicit(&scheduler.worker_wakeups, 1, memory_order_relaxed);
      }
      pthread_mutex_unlock(&scheduler.mutex);
      continue;
    }
    execute_task(task);
  }
  current_worker = NULL;
  return NULL;
}

static void reset_metrics(void) {
  atomic_store(&scheduler.active_tasks, 0);
  atomic_store(&scheduler.peak_active_tasks, 0);
  atomic_store(&scheduler.runnable_tasks, 0);
  atomic_store(&scheduler.spawned_tasks, 0);
  atomic_store(&scheduler.completed_tasks, 0);
  atomic_store(&scheduler.steal_attempts, 0);
  atomic_store(&scheduler.successful_steals, 0);
  atomic_store(&scheduler.worker_parks, 0);
  atomic_store(&scheduler.worker_wakeups, 0);
}

int tsnative_scheduler_init(void) {
  pthread_mutex_lock(&scheduler.mutex);
  if (scheduler.started) {
    pthread_mutex_unlock(&scheduler.mutex);
    return 0;
  }
  scheduler.worker_count = configured_workers();
  scheduler.max_tasks = configured_max_tasks();
  scheduler.workers = calloc(scheduler.worker_count, sizeof(*scheduler.workers));
  if (!scheduler.workers) {
    pthread_mutex_unlock(&scheduler.mutex);
    return -1;
  }
  for (size_t i = 0; i < scheduler.worker_count; i++) {
    scheduler.workers[i].index = i;
    pthread_mutex_init(&scheduler.workers[i].deque_mutex, NULL);
  }
  scheduler.started = 1;
  scheduler.stopping = 0;
  scheduler.head = scheduler.tail = NULL;
  scheduler.next_task_id = 0;
  reset_metrics();
  if (tsnative_timer_bind_scheduler) {
    tsnative_timer_bind_scheduler(
        (uintptr_t)&tsnative_scheduler_current_task,
        (uintptr_t)&tsnative_scheduler_prepare_park,
        (uintptr_t)&tsnative_scheduler_cancel_park,
        (uintptr_t)&tsnative_scheduler_wake,
        (uintptr_t)&tsnative_scheduler_help_once);
  }
  if (tsnative_blocking_bind_scheduler) {
    tsnative_blocking_bind_scheduler(
        (uintptr_t)&tsnative_scheduler_current_task,
        (uintptr_t)&tsnative_scheduler_prepare_park,
        (uintptr_t)&tsnative_scheduler_cancel_park,
        (uintptr_t)&tsnative_scheduler_wake,
        (uintptr_t)&tsnative_scheduler_help_once);
  }
  if (tsnative_channel_bind_scheduler) {
    tsnative_channel_bind_scheduler(
        (uintptr_t)&tsnative_scheduler_current_task,
        (uintptr_t)&tsnative_scheduler_prepare_park,
        (uintptr_t)&tsnative_scheduler_cancel_park,
        (uintptr_t)&tsnative_scheduler_wake,
        (uintptr_t)&tsnative_scheduler_help_once);
  }

  size_t created = 0;
  for (; created < scheduler.worker_count; created++) {
    if (pthread_create(&scheduler.workers[created].thread, NULL, worker_main, &scheduler.workers[created]) != 0) break;
  }
  if (created == scheduler.worker_count) {
    pthread_mutex_unlock(&scheduler.mutex);
    return 0;
  }

  scheduler.stopping = 1;
  pthread_cond_broadcast(&scheduler.wake);
  pthread_mutex_unlock(&scheduler.mutex);
  for (size_t i = 0; i < created; i++) pthread_join(scheduler.workers[i].thread, NULL);

  pthread_mutex_lock(&scheduler.mutex);
  for (size_t i = 0; i < scheduler.worker_count; i++) pthread_mutex_destroy(&scheduler.workers[i].deque_mutex);
  free(scheduler.workers);
  scheduler.workers = NULL;
  scheduler.worker_count = 0;
  scheduler.started = 0;
  scheduler.stopping = 0;
  pthread_mutex_unlock(&scheduler.mutex);
  return -1;
}

int tsnative_scheduler_submit(tsnative_task *task) {
  if (!task) return -1;
  pthread_mutex_lock(&scheduler.mutex);
  size_t active = atomic_load_explicit(&scheduler.active_tasks, memory_order_relaxed);
  if (!scheduler.started || scheduler.stopping || active >= scheduler.max_tasks) {
    pthread_mutex_unlock(&scheduler.mutex);
    return -1;
  }

  task->id = ++scheduler.next_task_id;
  atomic_store_explicit(&task->status, TSNATIVE_TASK_RUNNABLE, memory_order_release);
  active = atomic_fetch_add_explicit(&scheduler.active_tasks, 1, memory_order_relaxed) + 1;
  atomic_fetch_add_explicit(&scheduler.runnable_tasks, 1, memory_order_relaxed);
  atomic_fetch_add_explicit(&scheduler.spawned_tasks, 1, memory_order_relaxed);
  size_t peak = atomic_load_explicit(&scheduler.peak_active_tasks, memory_order_relaxed);
  if (active > peak) atomic_store_explicit(&scheduler.peak_active_tasks, active, memory_order_relaxed);

  if (current_worker) worker_push_tail(current_worker, task);
  else injection_push_locked(task);
  pthread_cond_signal(&scheduler.wake);
  pthread_mutex_unlock(&scheduler.mutex);
  return 0;
}

int tsnative_scheduler_wait(tsnative_task *task) {
  if (!task) return -1;
  while (!terminal(atomic_load_explicit(&task->status, memory_order_acquire))) {
    if (current_worker) {
      tsnative_task *help = take_work(current_worker);
      if (help) {
        execute_task(help);
        continue;
      }
    }
    pthread_mutex_lock(&scheduler.mutex);
    if (!terminal(atomic_load_explicit(&task->status, memory_order_acquire))) {
      pthread_cond_wait(&scheduler.task_done, &scheduler.mutex);
    }
    pthread_mutex_unlock(&scheduler.mutex);
  }
  return atomic_load_explicit(&task->status, memory_order_acquire) == TSNATIVE_TASK_DONE ? 0 : -1;
}

tsnative_task_status tsnative_scheduler_task_status(tsnative_task *task) {
  if (!task) return TSNATIVE_TASK_FAILED;
  return (tsnative_task_status)atomic_load_explicit(&task->status, memory_order_acquire);
}

int tsnative_scheduler_help_once(void) {
  if (!current_worker) return 0;
  tsnative_task *task = take_work(current_worker);
  if (!task) return 0;
  execute_task(task);
  return 1;
}

void tsnative_scheduler_shutdown(void) {
  pthread_mutex_lock(&scheduler.mutex);
  if (!scheduler.started) {
    pthread_mutex_unlock(&scheduler.mutex);
    return;
  }
  scheduler.stopping = 1;
  pthread_cond_broadcast(&scheduler.wake);
  tsnative_worker *workers = scheduler.workers;
  size_t count = scheduler.worker_count;
  pthread_mutex_unlock(&scheduler.mutex);

  for (size_t i = 0; i < count; i++) pthread_join(workers[i].thread, NULL);

  pthread_mutex_lock(&scheduler.mutex);
  for (size_t i = 0; i < count; i++) pthread_mutex_destroy(&workers[i].deque_mutex);
  free(workers);
  scheduler.workers = NULL;
  scheduler.worker_count = 0;
  scheduler.started = 0;
  scheduler.stopping = 0;
  scheduler.head = scheduler.tail = NULL;
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

size_t tsnative_scheduler_active_tasks(void) { return atomic_load(&scheduler.active_tasks); }
size_t tsnative_scheduler_peak_active_tasks(void) { return atomic_load(&scheduler.peak_active_tasks); }
uint64_t tsnative_scheduler_spawned_tasks(void) { return atomic_load(&scheduler.spawned_tasks); }
uint64_t tsnative_scheduler_completed_tasks(void) { return atomic_load(&scheduler.completed_tasks); }
uint64_t tsnative_scheduler_steal_attempts(void) { return atomic_load(&scheduler.steal_attempts); }
uint64_t tsnative_scheduler_successful_steals(void) { return atomic_load(&scheduler.successful_steals); }
uint64_t tsnative_scheduler_worker_parks(void) { return atomic_load(&scheduler.worker_parks); }
uint64_t tsnative_scheduler_worker_wakeups(void) { return atomic_load(&scheduler.worker_wakeups); }

tsnative_task *tsnative_scheduler_current_task(void) { return current_task; }

int tsnative_scheduler_prepare_park(void) {
  if (!current_task) return -1;
  atomic_store_explicit(&current_task->park_requested, 1, memory_order_release);
  return 0;
}

void tsnative_scheduler_cancel_park(void) {
  if (!current_task) return;
  atomic_store_explicit(&current_task->park_requested, 0, memory_order_release);
  atomic_store_explicit(&current_task->wake_requested, 0, memory_order_release);
}

int tsnative_scheduler_wake(tsnative_task *task) {
  if (!task) return -1;
  pthread_mutex_lock(&scheduler.mutex);
  int status = atomic_load_explicit(&task->status, memory_order_acquire);
  if (status == TSNATIVE_TASK_WAITING) {
    atomic_store_explicit(&task->status, TSNATIVE_TASK_RUNNABLE, memory_order_release);
    injection_push_locked(task);
    atomic_fetch_add_explicit(&scheduler.runnable_tasks, 1, memory_order_relaxed);
    pthread_cond_signal(&scheduler.wake);
    pthread_mutex_unlock(&scheduler.mutex);
    return 0;
  }
  if (status == TSNATIVE_TASK_RUNNING && atomic_load_explicit(&task->park_requested, memory_order_acquire)) {
    atomic_store_explicit(&task->wake_requested, 1, memory_order_release);
    pthread_mutex_unlock(&scheduler.mutex);
    return 0;
  }
  pthread_mutex_unlock(&scheduler.mutex);
  return status == TSNATIVE_TASK_RUNNABLE ? 0 : -1;
}
