#ifndef TSNATIVE_TASK_H
#define TSNATIVE_TASK_H

#include <stdint.h>

typedef struct tsnative_task tsnative_task;
typedef void (*tsnative_task_entry)(void *state, void *result_slot);

typedef enum {
  TSNATIVE_TASK_RUNNABLE = 1,
  TSNATIVE_TASK_RUNNING,
  TSNATIVE_TASK_WAITING,
  TSNATIVE_TASK_DONE,
  TSNATIVE_TASK_CANCELLED,
  TSNATIVE_TASK_FAILED,
} tsnative_task_status;

typedef enum {
  TSNATIVE_TASK_RESULT_VOID = 0,
  TSNATIVE_TASK_RESULT_F64 = 1,
  TSNATIVE_TASK_RESULT_REF = 2,
} tsnative_task_result_kind;

tsnative_task *tsnative_task_spawn(tsnative_task_entry entry, void *state);
tsnative_task *tsnative_task_spawn_or_abort(tsnative_task_entry entry, void *state);
tsnative_task *tsnative_task_spawn_f64(tsnative_task_entry entry, void *state);
tsnative_task *tsnative_task_spawn_f64_or_abort(tsnative_task_entry entry, void *state);
tsnative_task *tsnative_task_spawn_ref(tsnative_task_entry entry, void *state);
tsnative_task *tsnative_task_spawn_ref_or_abort(tsnative_task_entry entry, void *state);
int tsnative_task_join(tsnative_task *task);
double tsnative_task_join_f64_release(tsnative_task *task);
void *tsnative_task_join_ref_release(tsnative_task *task);
void tsnative_task_join_release(tsnative_task *task);
void tsnative_task_release(tsnative_task *task);
void tsnative_task_yield(void);
tsnative_task_status tsnative_task_get_status(tsnative_task *task);

#endif
