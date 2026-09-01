#include "channel_f64.h"
#include "scheduler_internal.h"
#include "../core/heap.h"

#include <pthread.h>
#include <stdlib.h>

struct tsnative_channel_f64 {
  pthread_mutex_t mutex;
  pthread_cond_t changed;
  size_t capacity;
  size_t head;
  size_t tail;
  size_t count;
  int has_value;
  double slot;
  double buffer[];
};

static int help_scheduler(tsnative_channel_f64 *channel) {
  pthread_mutex_unlock(&channel->mutex);
  int helped = tsnative_scheduler_help_once();
  pthread_mutex_lock(&channel->mutex);
  return helped;
}

tsnative_channel_f64 *tsnative_channel_f64_new(size_t capacity) {
  if (capacity > (SIZE_MAX - sizeof(tsnative_channel_f64)) / sizeof(double)) abort();
  tsnative_channel_f64 *channel = tsnative_heap_alloc(sizeof(*channel) + capacity * sizeof(double));
  channel->capacity = capacity;
  channel->head = channel->tail = channel->count = 0;
  channel->has_value = 0;
  channel->slot = 0;
  if (pthread_mutex_init(&channel->mutex, NULL) != 0) abort();
  if (pthread_cond_init(&channel->changed, NULL) != 0) abort();
  return channel;
}

int tsnative_channel_f64_try_send(tsnative_channel_f64 *channel, double value) {
  if (!channel) return -1;
  pthread_mutex_lock(&channel->mutex);
  int result = 0;
  if (channel->capacity == 0) {
    if (channel->has_value) {
      result = 0;
    } else {
      channel->slot = value;
      channel->has_value = 1;
      pthread_cond_broadcast(&channel->changed);
      result = 1;
    }
  } else if (channel->count == channel->capacity) {
    result = 0;
  } else {
    channel->buffer[channel->tail] = value;
    channel->tail = (channel->tail + 1) % channel->capacity;
    channel->count++;
    pthread_cond_broadcast(&channel->changed);
    result = 1;
  }
  pthread_mutex_unlock(&channel->mutex);
  return result;
}

int tsnative_channel_f64_try_recv(tsnative_channel_f64 *channel, double *out) {
  if (!channel || !out) return -1;
  pthread_mutex_lock(&channel->mutex);
  int result = 0;
  if (channel->capacity == 0) {
    if (channel->has_value) {
      *out = channel->slot;
      channel->has_value = 0;
      pthread_cond_broadcast(&channel->changed);
      result = 1;
    }
  } else if (channel->count != 0) {
    *out = channel->buffer[channel->head];
    channel->head = (channel->head + 1) % channel->capacity;
    channel->count--;
    pthread_cond_broadcast(&channel->changed);
    result = 1;
  }
  pthread_mutex_unlock(&channel->mutex);
  return result;
}

void tsnative_channel_f64_send(tsnative_channel_f64 *channel, double value) {
  if (!channel) abort();
  pthread_mutex_lock(&channel->mutex);
  if (channel->capacity == 0) {
    while (channel->has_value) {
      if (help_scheduler(channel)) continue;
      if (channel->has_value) pthread_cond_wait(&channel->changed, &channel->mutex);
    }
    channel->slot = value;
    channel->has_value = 1;
    pthread_cond_broadcast(&channel->changed);
    while (channel->has_value) {
      if (help_scheduler(channel)) continue;
      if (channel->has_value) pthread_cond_wait(&channel->changed, &channel->mutex);
    }
    pthread_mutex_unlock(&channel->mutex);
    return;
  }
  while (channel->count == channel->capacity) {
    if (help_scheduler(channel)) continue;
    if (channel->count == channel->capacity) pthread_cond_wait(&channel->changed, &channel->mutex);
  }
  channel->buffer[channel->tail] = value;
  channel->tail = (channel->tail + 1) % channel->capacity;
  channel->count++;
  pthread_cond_broadcast(&channel->changed);
  pthread_mutex_unlock(&channel->mutex);
}

double tsnative_channel_f64_recv(tsnative_channel_f64 *channel) {
  if (!channel) abort();
  pthread_mutex_lock(&channel->mutex);
  if (channel->capacity == 0) {
    while (!channel->has_value) {
      if (help_scheduler(channel)) continue;
      if (!channel->has_value) pthread_cond_wait(&channel->changed, &channel->mutex);
    }
    double value = channel->slot;
    channel->has_value = 0;
    pthread_cond_broadcast(&channel->changed);
    pthread_mutex_unlock(&channel->mutex);
    return value;
  }
  while (channel->count == 0) {
    if (help_scheduler(channel)) continue;
    if (channel->count == 0) pthread_cond_wait(&channel->changed, &channel->mutex);
  }
  double value = channel->buffer[channel->head];
  channel->head = (channel->head + 1) % channel->capacity;
  channel->count--;
  pthread_cond_broadcast(&channel->changed);
  pthread_mutex_unlock(&channel->mutex);
  return value;
}
