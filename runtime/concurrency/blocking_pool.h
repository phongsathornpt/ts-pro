#ifndef TSNATIVE_BLOCKING_POOL_H
#define TSNATIVE_BLOCKING_POOL_H

#include <stddef.h>
#include <stdint.h>

typedef struct tsnative_blocking_job tsnative_blocking_job;
typedef void (*tsnative_blocking_entry)(void *state);

tsnative_blocking_job *tsnative_blocking_submit(tsnative_blocking_entry entry, void *state);
int tsnative_blocking_job_wait_task(tsnative_blocking_job *job);
void tsnative_blocking_job_wait_cooperative(tsnative_blocking_job *job);
void tsnative_blocking_job_release(tsnative_blocking_job *job);
void tsnative_blocking_pool_shutdown(void);

size_t tsnative_blocking_worker_count(void);
size_t tsnative_blocking_active_jobs(void);
size_t tsnative_blocking_peak_active_jobs(void);
uint64_t tsnative_blocking_submitted_jobs(void);
uint64_t tsnative_blocking_completed_jobs(void);

#endif
