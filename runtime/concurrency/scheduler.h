#ifndef TSNATIVE_SCHEDULER_H
#define TSNATIVE_SCHEDULER_H

#include <stddef.h>
#include <stdint.h>

int tsnative_scheduler_init(void);
void tsnative_scheduler_shutdown(void);
size_t tsnative_scheduler_worker_count(void);
int tsnative_scheduler_is_running(void);

size_t tsnative_scheduler_active_tasks(void);
size_t tsnative_scheduler_peak_active_tasks(void);
uint64_t tsnative_scheduler_spawned_tasks(void);
uint64_t tsnative_scheduler_completed_tasks(void);

#endif
