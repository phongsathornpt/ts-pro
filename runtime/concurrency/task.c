#include "task.h"
#include "scheduler.h"
#include "scheduler_internal.h"
#include "task_internal.h"
#include "../core/heap.h"

#include <sched.h>
#include <stdlib.h>

extern void tsnative_console_log_jsvalue(void *value);

static void destroy_task_storage(tsnative_task *task);

struct tsnative_task_group {
  pthread_mutex_t mutex;
  pthread_cond_t done;
  tsnative_task *head;
  size_t active;
  int closed;
};

static void task_group_completed(tsnative_task *task) {
  tsnative_task_group *group = task ? task->group : NULL;
  if (!group) return;
  pthread_mutex_lock(&group->mutex);
  tsnative_task **cursor = &group->head;
  while (*cursor && *cursor != task) cursor = &(*cursor)->group_next;
  if (*cursor == task) {
    *cursor = task->group_next;
    task->group_next = NULL;
    task->group = NULL;
    if (group->active) group->active--;
    pthread_cond_broadcast(&group->done);
  }
  pthread_mutex_unlock(&group->mutex);
}

static int task_group_attach(tsnative_task_group *group, tsnative_task *task) {
  if (!group) return 0;
  pthread_mutex_lock(&group->mutex);
  if (group->closed) { pthread_mutex_unlock(&group->mutex); return -1; }
  task->group = group;
  task->group_next = group->head;
  task->notify_completed = task_group_completed;
  group->head = task;
  group->active++;
  pthread_mutex_unlock(&group->mutex);
  return 0;
}

static void task_group_detach_submit_failure(tsnative_task *task) {
  if (!task || !task->group) return;
  task_group_completed(task);
}


static void transfer_f64(tsnative_task *task, void *out) { *(double *)out = task->result.f64; }
static void transfer_bool(tsnative_task *task, void *out) { *(uint8_t *)out = task->result.boolean; }
static void transfer_ref(tsnative_task *task, void *out) {
  tsnative_gc_handoff_begin();
  *(void **)out = task->result.ref;
  task->completion_handoff = 1;
}

static tsnative_task *create_task_storage(tsnative_task_entry entry, void *state, tsnative_task_result_kind kind) {
  if (!entry) return NULL;
  tsnative_task *task = calloc(1, sizeof(*task));
  if (!task) return NULL;
  if (pthread_mutex_init(&task->completion_mutex, NULL) != 0) { free(task); return NULL; }
  task->entry = entry;
  task->state = state;
  task->result_kind = kind;
  tsnative_task *parent = tsnative_scheduler_current_task();
  if (parent) task->context = parent->context;
  atomic_store_explicit(&task->budget_remaining, 256u, memory_order_relaxed);
  task->destroy_completed = destroy_task_storage;
  if (kind == TSNATIVE_TASK_RESULT_F64) task->transfer_completion = transfer_f64;
  else if (kind == TSNATIVE_TASK_RESULT_BOOL) task->transfer_completion = transfer_bool;
  else if (kind == TSNATIVE_TASK_RESULT_REF) task->transfer_completion = transfer_ref;
  task->context_gc_root_token = tsnative_gc_root_register(&task->context);
  if (!task->context_gc_root_token) { pthread_mutex_destroy(&task->completion_mutex); free(task); return NULL; }
  if (kind == TSNATIVE_TASK_RESULT_REF) {
    task->result_gc_root_token = tsnative_gc_root_register(&task->result.ref);
    if (!task->result_gc_root_token) {
      if (task->context_gc_root_token) tsnative_gc_root_unregister(task->context_gc_root_token);
      pthread_mutex_destroy(&task->completion_mutex);
      free(task);
      return NULL;
    }
  }
  if (state) {
    task->gc_root_token = tsnative_gc_root_register(&task->state);
    if (!task->gc_root_token) {
      if (task->result_gc_root_token) tsnative_gc_root_unregister(task->result_gc_root_token);
      if (task->context_gc_root_token) tsnative_gc_root_unregister(task->context_gc_root_token);
      pthread_mutex_destroy(&task->completion_mutex);
      free(task);
      return NULL;
    }
  }
  task->failure_gc_root_token = tsnative_gc_root_register(&task->failure_ref);
  if (!task->failure_gc_root_token) {
    if (task->gc_root_token) tsnative_gc_root_unregister(task->gc_root_token);
    if (task->result_gc_root_token) tsnative_gc_root_unregister(task->result_gc_root_token);
    if (task->context_gc_root_token) tsnative_gc_root_unregister(task->context_gc_root_token);
    pthread_mutex_destroy(&task->completion_mutex);
    free(task);
    return NULL;
  }
  task->status = TSNATIVE_TASK_RUNNABLE;
  return task;
}

tsnative_task *tsnative_task_create_unsubmitted_internal(tsnative_task_entry entry, void *state, tsnative_task_result_kind kind) {
  return create_task_storage(entry, state, kind);
}

void tsnative_task_destroy_unsubmitted_internal(tsnative_task *task) {
  destroy_task_storage(task);
}

static tsnative_task *spawn_with_kind_group(tsnative_task_group *group, tsnative_task_entry entry, void *state, tsnative_task_result_kind kind) {
  if (tsnative_scheduler_init() != 0) return NULL;
  tsnative_task *task = create_task_storage(entry, state, kind);
  if (!task) return NULL;
  if (task_group_attach(group, task) != 0) {
    destroy_task_storage(task);
    return NULL;
  }
  if (tsnative_scheduler_submit(task) != 0) {
    task_group_detach_submit_failure(task);
    destroy_task_storage(task);
    return NULL;
  }
  return task;
}

tsnative_task *tsnative_task_spawn(tsnative_task_entry entry, void *state) {
  return spawn_with_kind_group(NULL, entry, state, TSNATIVE_TASK_RESULT_VOID);
}

tsnative_task *tsnative_task_spawn_or_abort(tsnative_task_entry entry, void *state) {
  tsnative_task *task = tsnative_task_spawn(entry, state);
  if (!task) abort();
  return task;
}

tsnative_task *tsnative_task_spawn_f64(tsnative_task_entry entry, void *state) {
  return spawn_with_kind_group(NULL, entry, state, TSNATIVE_TASK_RESULT_F64);
}

tsnative_task *tsnative_task_spawn_f64_or_abort(tsnative_task_entry entry, void *state) {
  tsnative_task *task = tsnative_task_spawn_f64(entry, state);
  if (!task) abort();
  return task;
}

tsnative_task *tsnative_task_spawn_bool(tsnative_task_entry entry, void *state) {
  return spawn_with_kind_group(NULL, entry, state, TSNATIVE_TASK_RESULT_BOOL);
}

tsnative_task *tsnative_task_spawn_bool_or_abort(tsnative_task_entry entry, void *state) {
  tsnative_task *task = tsnative_task_spawn_bool(entry, state);
  if (!task) abort();
  return task;
}

tsnative_task *tsnative_task_spawn_ref(tsnative_task_entry entry, void *state) {
  return spawn_with_kind_group(NULL, entry, state, TSNATIVE_TASK_RESULT_REF);
}

tsnative_task *tsnative_task_spawn_ref_or_abort(tsnative_task_entry entry, void *state) {
  tsnative_task *task = tsnative_task_spawn_ref(entry, state);
  if (!task) abort();
  return task;
}

__attribute__((weak)) tsnative_task_group *tsnative_task_group_new(void) {
  tsnative_task_group *group = calloc(1, sizeof(*group));
  if (!group) return NULL;
  if (pthread_mutex_init(&group->mutex, NULL) != 0) { free(group); return NULL; }
  if (pthread_cond_init(&group->done, NULL) != 0) { pthread_mutex_destroy(&group->mutex); free(group); return NULL; }
  return group;
}

static tsnative_task *group_spawn_or_abort(tsnative_task_group *group, tsnative_task_entry entry, void *state, tsnative_task_result_kind kind) {
  tsnative_task *task = spawn_with_kind_group(group, entry, state, kind);
  if (!task) abort();
  return task;
}

__attribute__((weak)) tsnative_task *tsnative_task_group_spawn_or_abort(tsnative_task_group *group, tsnative_task_entry entry, void *state) { return group_spawn_or_abort(group, entry, state, TSNATIVE_TASK_RESULT_VOID); }
__attribute__((weak)) tsnative_task *tsnative_task_group_spawn_f64_or_abort(tsnative_task_group *group, tsnative_task_entry entry, void *state) { return group_spawn_or_abort(group, entry, state, TSNATIVE_TASK_RESULT_F64); }
__attribute__((weak)) tsnative_task *tsnative_task_group_spawn_bool_or_abort(tsnative_task_group *group, tsnative_task_entry entry, void *state) { return group_spawn_or_abort(group, entry, state, TSNATIVE_TASK_RESULT_BOOL); }
__attribute__((weak)) tsnative_task *tsnative_task_group_spawn_ref_or_abort(tsnative_task_group *group, tsnative_task_entry entry, void *state) { return group_spawn_or_abort(group, entry, state, TSNATIVE_TASK_RESULT_REF); }

__attribute__((weak)) int tsnative_task_group_cancel(tsnative_task_group *group) {
  if (!group) return -1;
  pthread_mutex_lock(&group->mutex);
  for (tsnative_task *task = group->head; task; task = task->group_next) atomic_store_explicit(&task->cancel_requested, 1, memory_order_release);
  pthread_mutex_unlock(&group->mutex);
  return 0;
}

__attribute__((weak)) int tsnative_task_group_join_release(tsnative_task_group *group) {
  if (!group) return -1;
  pthread_mutex_lock(&group->mutex);
  group->closed = 1;
  while (group->active != 0) pthread_cond_wait(&group->done, &group->mutex);
  pthread_mutex_unlock(&group->mutex);
  pthread_cond_destroy(&group->done);
  pthread_mutex_destroy(&group->mutex);
  free(group);
  return 0;
}

int tsnative_task_join(tsnative_task *task) {
  if (!task) return -1;
  return tsnative_scheduler_wait(task);
}

int tsnative_task_get_failure(tsnative_task *task, void **out) {
  if (!task || !out) return -1;
  *out = NULL;
  (void)tsnative_scheduler_wait(task);
  pthread_mutex_lock(&task->completion_mutex);
  int status = atomic_load_explicit(&task->status, memory_order_acquire);
  if (status == TSNATIVE_TASK_FAILED && task->failure_ref) {
    *out = task->failure_ref;
    pthread_mutex_unlock(&task->completion_mutex);
    return 1;
  }
  pthread_mutex_unlock(&task->completion_mutex);
  return status == TSNATIVE_TASK_DONE ? 0 : -1;
}

uint8_t tsnative_task_wait_status(tsnative_task *task) {
  if (!task) return 0;
  return tsnative_scheduler_wait(task) == 0 ? 1 : 0;
}

void *tsnative_task_failure_ref(tsnative_task *task) {
  void *failure = NULL;
  return tsnative_task_get_failure(task, &failure) == 1 ? failure : NULL;
}

int tsnative_task_await_status_task(tsnative_task *task, uint8_t *success) {
  if (!task || !success) return -1;
  tsnative_task *waiter = tsnative_scheduler_current_task();
  if (!waiter || waiter == task) return -1;
  pthread_mutex_lock(&task->completion_mutex);
  int status = atomic_load_explicit(&task->status, memory_order_acquire);
  if (status == TSNATIVE_TASK_DONE || status == TSNATIVE_TASK_FAILED || status == TSNATIVE_TASK_CANCELLED) {
    *success = status == TSNATIVE_TASK_DONE ? 1 : 0;
    pthread_mutex_unlock(&task->completion_mutex);
    return 1;
  }
  if (task->completion_waiter) {
    pthread_mutex_unlock(&task->completion_mutex);
    return -1;
  }
  if (tsnative_scheduler_prepare_park() != 0) {
    pthread_mutex_unlock(&task->completion_mutex);
    return -1;
  }
  task->completion_waiter = waiter;
  task->completion_out = success;
  task->completion_status_only = 1;
  pthread_mutex_unlock(&task->completion_mutex);
  return 0;
}

int tsnative_task_await_task(tsnative_task *task) {
  if (!task) return -1;
  tsnative_task *waiter = tsnative_scheduler_current_task();
  if (!waiter || waiter == task) return -1;
  pthread_mutex_lock(&task->completion_mutex);
  int status = atomic_load_explicit(&task->status, memory_order_acquire);
  if (status == TSNATIVE_TASK_DONE) {
    pthread_mutex_unlock(&task->completion_mutex);
    return 1;
  }
  if (status == TSNATIVE_TASK_CANCELLED || status == TSNATIVE_TASK_FAILED || task->completion_waiter) {
    pthread_mutex_unlock(&task->completion_mutex);
    return -1;
  }
  if (tsnative_scheduler_prepare_park() != 0) {
    pthread_mutex_unlock(&task->completion_mutex);
    return -1;
  }
  task->completion_waiter = waiter;
  pthread_mutex_unlock(&task->completion_mutex);
  return 0;
}

static int await_typed_task(tsnative_task *task, tsnative_task_result_kind kind, void *out) {
  if (!task || !out || task->result_kind != kind || !task->transfer_completion) return -1;
  tsnative_task *waiter = tsnative_scheduler_current_task();
  if (!waiter || waiter == task) return -1;
  pthread_mutex_lock(&task->completion_mutex);
  int status = atomic_load_explicit(&task->status, memory_order_acquire);
  if (status == TSNATIVE_TASK_DONE) {
    task->transfer_completion(task, out);
    pthread_mutex_unlock(&task->completion_mutex);
    destroy_task_storage(task);
    return 1;
  }
  if (status == TSNATIVE_TASK_CANCELLED || status == TSNATIVE_TASK_FAILED || task->completion_waiter) {
    pthread_mutex_unlock(&task->completion_mutex);
    return -1;
  }
  if (tsnative_scheduler_prepare_park() != 0) {
    pthread_mutex_unlock(&task->completion_mutex);
    return -1;
  }
  task->completion_waiter = waiter;
  task->completion_out = out;
  task->completion_consume = 1;
  pthread_mutex_unlock(&task->completion_mutex);
  return 0;
}

int tsnative_task_await_f64_task(tsnative_task *task, double *out) {
  return await_typed_task(task, TSNATIVE_TASK_RESULT_F64, out);
}

int tsnative_task_await_bool_task(tsnative_task *task, uint8_t *out) {
  return await_typed_task(task, TSNATIVE_TASK_RESULT_BOOL, out);
}

int tsnative_task_await_ref_task(tsnative_task *task, void **out) {
  return await_typed_task(task, TSNATIVE_TASK_RESULT_REF, out);
}

static void destroy_task_storage(tsnative_task *task) {
  if (!task) return;
  if (task->gc_root_token) tsnative_gc_root_unregister(task->gc_root_token);
  if (task->result_gc_root_token) tsnative_gc_root_unregister(task->result_gc_root_token);
  if (task->context_gc_root_token) tsnative_gc_root_unregister(task->context_gc_root_token);
  if (task->failure_gc_root_token) tsnative_gc_root_unregister(task->failure_gc_root_token);
  if (task->completion_handoff) {
    task->completion_handoff = 0;
    tsnative_gc_handoff_end();
  }
  pthread_mutex_destroy(&task->completion_mutex);
  free(task);
}

void tsnative_task_release(tsnative_task *task) {
  if (!task) return;
  (void)tsnative_scheduler_wait(task);
  destroy_task_storage(task);
}

static void abort_task_failure(tsnative_task *task) {
  if (task && task->failure_ref) tsnative_console_log_jsvalue(task->failure_ref);
  abort();
}

void tsnative_task_join_release(tsnative_task *task) {
  if (!task || task->result_kind != TSNATIVE_TASK_RESULT_VOID) abort();
  if (tsnative_task_join(task) != 0) abort_task_failure(task);
  tsnative_task_release(task);
}

double tsnative_task_join_f64_release(tsnative_task *task) {
  if (!task || task->result_kind != TSNATIVE_TASK_RESULT_F64) abort();
  if (tsnative_task_join(task) != 0) abort_task_failure(task);
  double result = task->result.f64;
  tsnative_task_release(task);
  return result;
}

uint8_t tsnative_task_join_bool_release(tsnative_task *task) {
  if (!task || task->result_kind != TSNATIVE_TASK_RESULT_BOOL) abort();
  if (tsnative_task_join(task) != 0) abort_task_failure(task);
  uint8_t result = task->result.boolean;
  tsnative_task_release(task);
  return result;
}

void *tsnative_task_join_ref_release(tsnative_task *task) {
  if (!task || task->result_kind != TSNATIVE_TASK_RESULT_REF) abort();
  if (tsnative_task_join(task) != 0) abort_task_failure(task);
  void *result = task->result.ref;
  tsnative_task_release(task);
  return result;
}

tsnative_task_status tsnative_task_get_status(tsnative_task *task) {
  if (!task) return TSNATIVE_TASK_FAILED;
  return tsnative_scheduler_task_status(task);
}

void tsnative_task_set_context(void *value) {
  tsnative_task *task = tsnative_scheduler_current_task();
  if (!task) abort();
  task->context = value;
}

void *tsnative_task_get_context(void) {
  tsnative_task *task = tsnative_scheduler_current_task();
  if (!task) abort();
  return task->context;
}

void tsnative_task_fail_current(void *error) {
  tsnative_task *task = tsnative_scheduler_current_task();
  if (!task) abort();
  task->failure_ref = error;
  atomic_store_explicit(&task->failure_requested, 1, memory_order_release);
}

int tsnative_task_cancel(tsnative_task *task) {
  if (!task) return -1;
  atomic_store_explicit(&task->cancel_requested, 1, memory_order_release);
  return 0;
}

int tsnative_task_is_cancelled(void) {
  tsnative_task *task = tsnative_scheduler_current_task();
  if (!task) return 0;
  return atomic_load_explicit(&task->cancel_requested, memory_order_acquire) ? 1 : 0;
}

int tsnative_task_budget_poll_task(void) {
  tsnative_task *task = tsnative_scheduler_current_task();
  if (!task) return 1;
  uint32_t previous = atomic_fetch_sub_explicit(&task->budget_remaining, 1u, memory_order_relaxed);
  if (previous > 1u) return 1;
  atomic_store_explicit(&task->budget_remaining, 256u, memory_order_relaxed);
  return tsnative_task_yield_task() == 0 ? 0 : -1;
}

int tsnative_task_yield_task(void) {
  tsnative_task *task = tsnative_scheduler_current_task();
  if (!task) return -1;
  if (tsnative_scheduler_prepare_park() != 0) return -1;
  if (tsnative_scheduler_wake(task) != 0) {
    tsnative_scheduler_cancel_park();
    return -1;
  }
  return 0;
}

void tsnative_task_yield(void) {
  sched_yield();
}
