#ifndef TSNATIVE_TASK_INTERNAL_H
#define TSNATIVE_TASK_INTERNAL_H

#include "task.h"

struct tsnative_task {
  struct tsnative_task *next;
  tsnative_task_entry entry;
  void *state;
  uint64_t id;
  tsnative_task_status status;
};

#endif
