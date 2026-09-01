#ifndef TSNATIVE_TASK_INTERNAL_H
#define TSNATIVE_TASK_INTERNAL_H

#include "task.h"
#include <stdatomic.h>

struct tsnative_task {
  struct tsnative_task *next;
  struct tsnative_task *prev;
  tsnative_task_entry entry;
  void *state;
  void *gc_root_token;
  void *result_gc_root_token;
  uint64_t id;
  _Atomic int status;
  tsnative_task_result_kind result_kind;
  union {
    double f64;
    void *ref;
  } result;
};

#endif
