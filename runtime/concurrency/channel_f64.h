#ifndef TSNATIVE_CHANNEL_F64_H
#define TSNATIVE_CHANNEL_F64_H

#include <stddef.h>

typedef struct tsnative_channel_f64 tsnative_channel_f64;

tsnative_channel_f64 *tsnative_channel_f64_new(size_t capacity);
void tsnative_channel_f64_send(tsnative_channel_f64 *channel, double value);
double tsnative_channel_f64_recv(tsnative_channel_f64 *channel);

#endif
