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
  uint64_t id;
  _Atomic int status;
  _Atomic int park_requested;
  _Atomic int wake_requested;
  _Atomic int cancel_requested;
  _Atomic uint32_t budget_remaining;
  pthread_mutex_t completion_mutex;
  struct tsnative_task *completion_waiter;
  void *completion_out;
  int completion_consume;
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


#endif
