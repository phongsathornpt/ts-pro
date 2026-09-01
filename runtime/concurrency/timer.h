#ifndef TSNATIVE_TIMER_H
#define TSNATIVE_TIMER_H

int tsnative_sleep_task(double milliseconds);
void tsnative_sleep_cooperative(double milliseconds);
void tsnative_timer_shutdown(void);

#endif
