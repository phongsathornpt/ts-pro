#ifndef TSNATIVE_CHANNEL_F64_H
#define TSNATIVE_CHANNEL_F64_H

#include <stddef.h>

typedef struct tsnative_channel_f64 tsnative_channel_f64;

tsnative_channel_f64 *tsnative_channel_f64_new(size_t capacity);
int tsnative_channel_f64_try_send(tsnative_channel_f64 *channel, double value);
int tsnative_channel_f64_try_recv(tsnative_channel_f64 *channel, double *out);
int tsnative_channel_f64_send_task(tsnative_channel_f64 *channel, double value);
int tsnative_channel_f64_recv_task(tsnative_channel_f64 *channel, double *out);
void tsnative_channel_f64_send(tsnative_channel_f64 *channel, double value);
double tsnative_channel_f64_recv(tsnative_channel_f64 *channel);

#endif
