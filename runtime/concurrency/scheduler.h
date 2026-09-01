#ifndef TSNATIVE_SCHEDULER_H
#define TSNATIVE_SCHEDULER_H

#include <stddef.h>

int tsnative_scheduler_init(void);
void tsnative_scheduler_shutdown(void);
size_t tsnative_scheduler_worker_count(void);
int tsnative_scheduler_is_running(void);

#endif
