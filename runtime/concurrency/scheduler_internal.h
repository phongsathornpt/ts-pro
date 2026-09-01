#ifndef TSNATIVE_SCHEDULER_INTERNAL_H
#define TSNATIVE_SCHEDULER_INTERNAL_H

#include "task_internal.h"

int tsnative_scheduler_submit(tsnative_task *task);
int tsnative_scheduler_wait(tsnative_task *task);
tsnative_task_status tsnative_scheduler_task_status(tsnative_task *task);
int tsnative_scheduler_help_once(void);
tsnative_task *tsnative_scheduler_current_task(void);
int tsnative_scheduler_prepare_park(void);
void tsnative_scheduler_cancel_park(void);
int tsnative_scheduler_wake(tsnative_task *task);

#endif
