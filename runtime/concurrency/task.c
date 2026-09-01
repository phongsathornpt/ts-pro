#include "task.h"
#include "scheduler.h"
#include "scheduler_internal.h"
#include "task_internal.h"
#include "../core/heap.h"

#include <sched.h>
#include <stdlib.h>

static tsnative_task *spawn_with_kind(tsnative_task_entry entry, void *state, tsnative_task_result_kind kind) {
  if (!entry) return NULL;
  if (tsnative_scheduler_init() != 0) return NULL;
  tsnative_task *task = calloc(1, sizeof(*task));
  if (!task) return NULL;
  task->entry = entry;
  task->state = state;
  task->result_kind = kind;
  if (kind == TSNATIVE_TASK_RESULT_REF) {
    task->result_gc_root_token = tsnative_gc_root_register(&task->result.ref);
    if (!task->result_gc_root_token) {
      free(task);
      return NULL;
    }
  }
  if (state) {
    task->gc_root_token = tsnative_gc_root_register(&task->state);
    if (!task->gc_root_token) {
      if (task->result_gc_root_token) tsnative_gc_root_unregister(task->result_gc_root_token);
      free(task);
      return NULL;
    }
  }
  task->status = TSNATIVE_TASK_RUNNABLE;
  if (tsnative_scheduler_submit(task) != 0) {
    if (task->gc_root_token) tsnative_gc_root_unregister(task->gc_root_token);
    if (task->result_gc_root_token) tsnative_gc_root_unregister(task->result_gc_root_token);
    free(task);
    return NULL;
  }
  return task;
}

tsnative_task *tsnative_task_spawn(tsnative_task_entry entry, void *state) {
  return spawn_with_kind(entry, state, TSNATIVE_TASK_RESULT_VOID);
}

tsnative_task *tsnative_task_spawn_or_abort(tsnative_task_entry entry, void *state) {
  tsnative_task *task = tsnative_task_spawn(entry, state);
  if (!task) abort();
  return task;
}

tsnative_task *tsnative_task_spawn_f64(tsnative_task_entry entry, void *state) {
  return spawn_with_kind(entry, state, TSNATIVE_TASK_RESULT_F64);
}

tsnative_task *tsnative_task_spawn_f64_or_abort(tsnative_task_entry entry, void *state) {
  tsnative_task *task = tsnative_task_spawn_f64(entry, state);
  if (!task) abort();
  return task;
}

tsnative_task *tsnative_task_spawn_bool(tsnative_task_entry entry, void *state) {
  return spawn_with_kind(entry, state, TSNATIVE_TASK_RESULT_BOOL);
}

tsnative_task *tsnative_task_spawn_bool_or_abort(tsnative_task_entry entry, void *state) {
  tsnative_task *task = tsnative_task_spawn_bool(entry, state);
  if (!task) abort();
  return task;
}

tsnative_task *tsnative_task_spawn_ref(tsnative_task_entry entry, void *state) {
  return spawn_with_kind(entry, state, TSNATIVE_TASK_RESULT_REF);
}

tsnative_task *tsnative_task_spawn_ref_or_abort(tsnative_task_entry entry, void *state) {
  tsnative_task *task = tsnative_task_spawn_ref(entry, state);
  if (!task) abort();
  return task;
}

int tsnative_task_join(tsnative_task *task) {
  if (!task) return -1;
  return tsnative_scheduler_wait(task);
}

void tsnative_task_release(tsnative_task *task) {
  if (!task) return;
  (void)tsnative_scheduler_wait(task);
  if (task->gc_root_token) tsnative_gc_root_unregister(task->gc_root_token);
  if (task->result_gc_root_token) tsnative_gc_root_unregister(task->result_gc_root_token);
  free(task);
}

void tsnative_task_join_release(tsnative_task *task) {
  if (!task || task->result_kind != TSNATIVE_TASK_RESULT_VOID || tsnative_task_join(task) != 0) abort();
  tsnative_task_release(task);
}

double tsnative_task_join_f64_release(tsnative_task *task) {
  if (!task || task->result_kind != TSNATIVE_TASK_RESULT_F64 || tsnative_task_join(task) != 0) abort();
  double result = task->result.f64;
  tsnative_task_release(task);
  return result;
}

uint8_t tsnative_task_join_bool_release(tsnative_task *task) {
  if (!task || task->result_kind != TSNATIVE_TASK_RESULT_BOOL || tsnative_task_join(task) != 0) abort();
  uint8_t result = task->result.boolean;
  tsnative_task_release(task);
  return result;
}

void *tsnative_task_join_ref_release(tsnative_task *task) {
  if (!task || task->result_kind != TSNATIVE_TASK_RESULT_REF || tsnative_task_join(task) != 0) abort();
  void *result = task->result.ref;
  tsnative_task_release(task);
  return result;
}

tsnative_task_status tsnative_task_get_status(tsnative_task *task) {
  if (!task) return TSNATIVE_TASK_FAILED;
  return tsnative_scheduler_task_status(task);
}

void tsnative_task_yield(void) {
  sched_yield();
}
