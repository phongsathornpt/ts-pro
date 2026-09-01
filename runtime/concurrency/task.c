#include "task.h"
#include "scheduler.h"
#include "scheduler_internal.h"
#include "task_internal.h"

#include <sched.h>
#include <stdlib.h>

tsnative_task *tsnative_task_spawn(tsnative_task_entry entry, void *state) {
  if (!entry) return NULL;
  if (tsnative_scheduler_init() != 0) return NULL;
  tsnative_task *task = calloc(1, sizeof(*task));
  if (!task) return NULL;
  task->entry = entry;
  task->state = state;
  task->status = TSNATIVE_TASK_RUNNABLE;
  if (tsnative_scheduler_submit(task) != 0) {
    free(task);
    return NULL;
  }
  return task;
}
tsnative_task *tsnative_task_spawn_or_abort(tsnative_task_entry entry, void *state) {
  tsnative_task *task = tsnative_task_spawn(entry, state);
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
  free(task);
}

void tsnative_task_join_release(tsnative_task *task) {
  if (tsnative_task_join(task) != 0) abort();
  tsnative_task_release(task);
}

tsnative_task_status tsnative_task_get_status(tsnative_task *task) {
  if (!task) return TSNATIVE_TASK_FAILED;
  return tsnative_scheduler_task_status(task);
}

void tsnative_task_yield(void) {
  sched_yield();
}
