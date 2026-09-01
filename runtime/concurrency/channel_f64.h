#ifndef TSNATIVE_CHANNEL_F64_H
#define TSNATIVE_CHANNEL_F64_H

#include <stddef.h>
#include <stdint.h>

#if defined(__GNUC__) || defined(__clang__)
void tsnative_channel_bind_scheduler(uintptr_t current, uintptr_t prepare, uintptr_t cancel, uintptr_t wake, uintptr_t help) __attribute__((weak));
#else
void tsnative_channel_bind_scheduler(uintptr_t current, uintptr_t prepare, uintptr_t cancel, uintptr_t wake, uintptr_t help);
#endif

typedef struct tsnative_channel_f64 tsnative_channel_f64;

tsnative_channel_f64 *tsnative_channel_f64_new(size_t capacity);
tsnative_channel_f64 *tsnative_channel_f64_new_checked(double capacity);
int tsnative_channel_f64_try_send(tsnative_channel_f64 *channel, double value);
int tsnative_channel_f64_try_recv(tsnative_channel_f64 *channel, double *out);
double tsnative_channel_f64_try_recv_or(tsnative_channel_f64 *channel, double fallback);
int tsnative_channel_f64_send_task(tsnative_channel_f64 *channel, double value);
int tsnative_channel_f64_recv_task(tsnative_channel_f64 *channel, double *out);
void tsnative_channel_f64_send(tsnative_channel_f64 *channel, double value);
double tsnative_channel_f64_recv(tsnative_channel_f64 *channel);
void tsnative_channel_f64_send_cooperative(tsnative_channel_f64 *channel, double value);
double tsnative_channel_f64_recv_cooperative(tsnative_channel_f64 *channel);

#endif
