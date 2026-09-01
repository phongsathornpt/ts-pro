#ifndef TSNATIVE_TASK_INTERNAL_H
#define TSNATIVE_TASK_INTERNAL_H

#include "task.h"
#include <stdatomic.h>
#include <pthread.h>

struct tsnative_task {
  struct tsnative_task *next;
  struct tsnative_task *prev;
  struct tsnative_task *group_next;
  tsnative_task_group *group;
  tsnative_task_entry entry;
  void *state;
  void *gc_root_token;
  void *result_gc_root_token;
  void *context;
  void *context_gc_root_token;
  void *failure_ref;
  void *failure_gc_root_token;
  uint64_t id;
  _Atomic int status;
  _Atomic int park_requested;
  _Atomic int wake_requested;
  _Atomic int cancel_requested;
  _Atomic int failure_requested;
  _Atomic uint32_t budget_remaining;
  pthread_mutex_t completion_mutex;
  struct tsnative_task *completion_waiter;
  void *completion_out;
  int completion_consume;
  int completion_status_only;
  int completion_handoff;
  void (*transfer_completion)(struct tsnative_task *task, void *out);
  void (*destroy_completed)(struct tsnative_task *task);
  void (*notify_completed)(struct tsnative_task *task);
  tsnative_task_result_kind result_kind;
  union {
    double f64;
    uint8_t boolean;
    void *ref;
  } result;
};


tsnative_task *tsnative_task_create_unsubmitted_internal(tsnative_task_entry entry, void *state, tsnative_task_result_kind kind);
void tsnative_task_destroy_unsubmitted_internal(tsnative_task *task);

static inline void tsnative_task_request_cancel_internal(tsnative_task *task) {
  if (task) atomic_store_explicit(&task->cancel_requested, 1, memory_order_release);
}

typedef enum {
  TSNATIVE_TASK_EXEC_WAITING = 0,
  TSNATIVE_TASK_EXEC_REQUEUE = 1,
  TSNATIVE_TASK_EXEC_TERMINAL = 2,
} tsnative_task_execution_kind;

typedef struct {
  tsnative_task_execution_kind kind;
  tsnative_task *completion_waiter;
  int completion_consume;
} tsnative_task_execution;

static inline void tsnative_task_mark_submitted_internal(tsnative_task *task, uint64_t id) {
  if (!task) return;
  task->id = id;
  atomic_store_explicit(&task->status, TSNATIVE_TASK_RUNNABLE, memory_order_release);
}

static inline int tsnative_task_is_terminal_internal(tsnative_task *task) {
  if (!task) return 1;
  int status = atomic_load_explicit(&task->status, memory_order_acquire);
  return status == TSNATIVE_TASK_DONE || status == TSNATIVE_TASK_CANCELLED || status == TSNATIVE_TASK_FAILED;
}

static inline tsnative_task_status tsnative_task_status_internal(tsnative_task *task) {
  if (!task) return TSNATIVE_TASK_FAILED;
  return (tsnative_task_status)atomic_load_explicit(&task->status, memory_order_acquire);
}

static inline int tsnative_task_prepare_park_internal(tsnative_task *task) {
  if (!task) return -1;
  atomic_store_explicit(&task->park_requested, 1, memory_order_release);
  return 0;
}

static inline void tsnative_task_cancel_park_internal(tsnative_task *task) {
  if (!task) return;
  atomic_store_explicit(&task->park_requested, 0, memory_order_release);
  atomic_store_explicit(&task->wake_requested, 0, memory_order_release);
}

static inline int tsnative_task_wake_internal(tsnative_task *task) {
  if (!task) return -1;
  int status = atomic_load_explicit(&task->status, memory_order_acquire);
  if (status == TSNATIVE_TASK_WAITING) {
    atomic_store_explicit(&task->status, TSNATIVE_TASK_RUNNABLE, memory_order_release);
    return 1;
  }
  if (status == TSNATIVE_TASK_RUNNING && atomic_load_explicit(&task->park_requested, memory_order_acquire)) {
    atomic_store_explicit(&task->wake_requested, 1, memory_order_release);
    return 0;
  }
  return status == TSNATIVE_TASK_RUNNABLE ? 0 : -1;
}

static inline tsnative_task_execution tsnative_task_execute_once_internal(tsnative_task *task) {
  tsnative_task_execution execution = { .kind = TSNATIVE_TASK_EXEC_TERMINAL, .completion_waiter = NULL, .completion_consume = 0 };
  if (!task) return execution;
  atomic_store_explicit(&task->status, TSNATIVE_TASK_RUNNING, memory_order_release);
  atomic_store_explicit(&task->park_requested, 0, memory_order_release);
  atomic_store_explicit(&task->wake_requested, 0, memory_order_release);
  if (!atomic_load_explicit(&task->failure_requested, memory_order_acquire)) {
    task->entry(task->state, &task->result);
  }
  if (atomic_load_explicit(&task->park_requested, memory_order_acquire)) {
    if (atomic_exchange_explicit(&task->wake_requested, 0, memory_order_acq_rel)) {
      atomic_store_explicit(&task->status, TSNATIVE_TASK_RUNNABLE, memory_order_release);
      execution.kind = TSNATIVE_TASK_EXEC_REQUEUE;
    } else {
      atomic_store_explicit(&task->status, TSNATIVE_TASK_WAITING, memory_order_release);
      execution.kind = TSNATIVE_TASK_EXEC_WAITING;
    }
    return execution;
  }
  pthread_mutex_lock(&task->completion_mutex);
  int failed = atomic_load_explicit(&task->failure_requested, memory_order_acquire);
  atomic_store_explicit(&task->status, failed ? TSNATIVE_TASK_FAILED : TSNATIVE_TASK_DONE, memory_order_release);
  execution.completion_waiter = task->completion_waiter;
  void *completion_out = task->completion_out;
  execution.completion_consume = task->completion_consume;
  int completion_status_only = task->completion_status_only;
  task->completion_waiter = NULL;
  task->completion_out = NULL;
  task->completion_consume = 0;
  task->completion_status_only = 0;
  if (execution.completion_waiter && completion_status_only && completion_out) {
    *(uint8_t *)completion_out = failed ? 0 : 1;
  } else if (execution.completion_waiter && failed) {
    execution.completion_waiter->failure_ref = task->failure_ref;
    atomic_store_explicit(&execution.completion_waiter->failure_requested, 1, memory_order_release);
  } else if (execution.completion_waiter && completion_out && task->transfer_completion) {
    task->transfer_completion(task, completion_out);
  }
  pthread_mutex_unlock(&task->completion_mutex);
  execution.kind = TSNATIVE_TASK_EXEC_TERMINAL;
  return execution;
}

static inline void tsnative_task_notify_completed_internal(tsnative_task *task) {
  if (task && task->notify_completed) task->notify_completed(task);
}

static inline void tsnative_task_destroy_completed_internal(tsnative_task *task, int consume) {
  if (consume && task && task->destroy_completed) task->destroy_completed(task);
}

#endif
