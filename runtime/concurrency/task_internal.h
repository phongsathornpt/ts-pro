#ifndef TSNATIVE_TASK_INTERNAL_H
#define TSNATIVE_TASK_INTERNAL_H

#include "task.h"
#include <stdatomic.h>

struct tsnative_task {
  struct tsnative_task *next;
  struct tsnative_task *prev;
  tsnative_task_entry entry;
  void *state;
  uint64_t id;
  _Atomic int status;
};

#endif
