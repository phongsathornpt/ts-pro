#ifndef TSNATIVE_TIMER_H
#define TSNATIVE_TIMER_H

#include <stdint.h>

#if defined(__GNUC__) || defined(__clang__)
void tsnative_timer_bind_scheduler(uintptr_t current, uintptr_t prepare, uintptr_t cancel, uintptr_t wake, uintptr_t help) __attribute__((weak));
#else
void tsnative_timer_bind_scheduler(uintptr_t current, uintptr_t prepare, uintptr_t cancel, uintptr_t wake, uintptr_t help);
#endif
int tsnative_sleep_task(double milliseconds);
void tsnative_sleep_cooperative(double milliseconds);
void tsnative_timer_shutdown(void);

#endif
