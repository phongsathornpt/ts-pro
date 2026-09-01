#include "channel_f64.h"
#include "scheduler_internal.h"
#include "../core/heap.h"

#include <pthread.h>
#include <stdatomic.h>
#include <stdlib.h>

typedef struct tsnative_channel_f64_waiter {
  struct tsnative_channel_f64_waiter *next;
  tsnative_task *task;
  double value;
  double *out;
  int cooperative;
  _Atomic int completed;
} tsnative_channel_f64_waiter;

struct tsnative_channel_f64 {
  pthread_mutex_t mutex;
  pthread_cond_t changed;
  size_t capacity;
  size_t head;
  size_t tail;
  size_t count;
  int has_value;
  double slot;
  tsnative_channel_f64_waiter *send_head;
  tsnative_channel_f64_waiter *send_tail;
  tsnative_channel_f64_waiter *recv_head;
  tsnative_channel_f64_waiter *recv_tail;
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
  channel->send_head = channel->send_tail = NULL;
  channel->recv_head = channel->recv_tail = NULL;
  if (pthread_mutex_init(&channel->mutex, NULL) != 0) abort();
  if (pthread_cond_init(&channel->changed, NULL) != 0) abort();
  return channel;
}

static void waiter_push(tsnative_channel_f64_waiter **head, tsnative_channel_f64_waiter **tail, tsnative_channel_f64_waiter *waiter) {
  waiter->next = NULL;
  if (*tail) (*tail)->next = waiter;
  else *head = waiter;
  *tail = waiter;
}

static tsnative_channel_f64_waiter *waiter_pop(tsnative_channel_f64_waiter **head, tsnative_channel_f64_waiter **tail) {
  tsnative_channel_f64_waiter *waiter = *head;
  if (!waiter) return NULL;
  *head = waiter->next;
  if (!*head) *tail = NULL;
  waiter->next = NULL;
  return waiter;
}

tsnative_channel_f64 *tsnative_channel_f64_new_checked(double capacity) {
  if (!(capacity >= 0.0) || capacity > (double)SIZE_MAX) abort();
  size_t native_capacity = (size_t)capacity;
  if ((double)native_capacity != capacity) abort();
  return tsnative_channel_f64_new(native_capacity);
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

double tsnative_channel_f64_try_recv_or(tsnative_channel_f64 *channel, double fallback) {
  double value = fallback;
  return tsnative_channel_f64_try_recv(channel, &value) == 1 ? value : fallback;
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

int tsnative_channel_f64_send_task(tsnative_channel_f64 *channel, double value) {
  if (!channel) return -1;
  tsnative_task *task = tsnative_scheduler_current_task();
  if (!task) return -1;
  pthread_mutex_lock(&channel->mutex);
  tsnative_channel_f64_waiter *receiver = waiter_pop(&channel->recv_head, &channel->recv_tail);
  if (receiver) {
    *receiver->out = value;
    pthread_mutex_unlock(&channel->mutex);
    (void)tsnative_scheduler_wake(receiver->task);
    free(receiver);
    return 1;
  }
  if (channel->capacity != 0 && channel->count < channel->capacity) {
    channel->buffer[channel->tail] = value;
    channel->tail = (channel->tail + 1) % channel->capacity;
    channel->count++;
    pthread_cond_broadcast(&channel->changed);
    pthread_mutex_unlock(&channel->mutex);
    return 1;
  }
  if (tsnative_scheduler_prepare_park() != 0) {
    pthread_mutex_unlock(&channel->mutex);
    return -1;
  }
  tsnative_channel_f64_waiter *waiter = calloc(1, sizeof(*waiter));
  if (!waiter) {
    tsnative_scheduler_cancel_park();
    pthread_mutex_unlock(&channel->mutex);
    return -1;
  }
  waiter->task = task;
  waiter->value = value;
  waiter_push(&channel->send_head, &channel->send_tail, waiter);
  pthread_mutex_unlock(&channel->mutex);
  return 0;
}

int tsnative_channel_f64_recv_task(tsnative_channel_f64 *channel, double *out) {
  if (!channel || !out) return -1;
  tsnative_task *task = tsnative_scheduler_current_task();
  if (!task) return -1;
  pthread_mutex_lock(&channel->mutex);
  tsnative_channel_f64_waiter *sender = NULL;
  if (channel->capacity != 0 && channel->count != 0) {
    *out = channel->buffer[channel->head];
    channel->head = (channel->head + 1) % channel->capacity;
    channel->count--;
    sender = waiter_pop(&channel->send_head, &channel->send_tail);
    if (sender) {
      channel->buffer[channel->tail] = sender->value;
      channel->tail = (channel->tail + 1) % channel->capacity;
      channel->count++;
    }
    pthread_cond_broadcast(&channel->changed);
    pthread_mutex_unlock(&channel->mutex);
    if (sender) {
      (void)tsnative_scheduler_wake(sender->task);
      free(sender);
    }
    return 1;
  }
  sender = waiter_pop(&channel->send_head, &channel->send_tail);
  if (sender) {
    *out = sender->value;
    pthread_mutex_unlock(&channel->mutex);
    (void)tsnative_scheduler_wake(sender->task);
    free(sender);
    return 1;
  }
  if (tsnative_scheduler_prepare_park() != 0) {
    pthread_mutex_unlock(&channel->mutex);
    return -1;
  }
  tsnative_channel_f64_waiter *waiter = calloc(1, sizeof(*waiter));
  if (!waiter) {
    tsnative_scheduler_cancel_park();
    pthread_mutex_unlock(&channel->mutex);
    return -1;
  }
  waiter->task = task;
  waiter->out = out;
  waiter_push(&channel->recv_head, &channel->recv_tail, waiter);
  pthread_mutex_unlock(&channel->mutex);
  return 0;
}

static void finish_waiter(tsnative_channel_f64 *channel, tsnative_channel_f64_waiter *waiter) {
  if (waiter->cooperative) {
    atomic_store_explicit(&waiter->completed, 1, memory_order_release);
    pthread_cond_broadcast(&channel->changed);
    return;
  }
  (void)tsnative_scheduler_wake(waiter->task);
  free(waiter);
}

static void wait_cooperatively(tsnative_channel_f64 *channel, tsnative_channel_f64_waiter *waiter) {
  for (;;) {
    if (atomic_load_explicit(&waiter->completed, memory_order_acquire)) return;
    if (tsnative_scheduler_help_once()) continue;
    pthread_mutex_lock(&channel->mutex);
    if (!atomic_load_explicit(&waiter->completed, memory_order_acquire)) {
      pthread_cond_wait(&channel->changed, &channel->mutex);
    }
    pthread_mutex_unlock(&channel->mutex);
  }
}

void tsnative_channel_f64_send_cooperative(tsnative_channel_f64 *channel, double value) {
  if (!channel) abort();
  if (!tsnative_scheduler_current_task()) { tsnative_channel_f64_send(channel, value); return; }
  pthread_mutex_lock(&channel->mutex);
  tsnative_channel_f64_waiter *receiver = waiter_pop(&channel->recv_head, &channel->recv_tail);
  if (receiver) {
    *receiver->out = value;
    finish_waiter(channel, receiver);
    pthread_mutex_unlock(&channel->mutex);
    return;
  }
  if (channel->capacity != 0 && channel->count < channel->capacity) {
    channel->buffer[channel->tail] = value;
    channel->tail = (channel->tail + 1) % channel->capacity;
    channel->count++;
    pthread_cond_broadcast(&channel->changed);
    pthread_mutex_unlock(&channel->mutex);
    return;
  }
  tsnative_channel_f64_waiter *waiter = calloc(1, sizeof(*waiter));
  if (!waiter) abort();
  waiter->task = tsnative_scheduler_current_task(); waiter->value = value; waiter->cooperative = 1;
  waiter_push(&channel->send_head, &channel->send_tail, waiter);
  pthread_mutex_unlock(&channel->mutex);
  wait_cooperatively(channel, waiter);
  free(waiter);
}

double tsnative_channel_f64_recv_cooperative(tsnative_channel_f64 *channel) {
  if (!channel) abort();
  if (!tsnative_scheduler_current_task()) return tsnative_channel_f64_recv(channel);
  double value = 0;
  pthread_mutex_lock(&channel->mutex);
  if (channel->capacity != 0 && channel->count != 0) {
    value = channel->buffer[channel->head]; channel->head = (channel->head + 1) % channel->capacity; channel->count--;
    tsnative_channel_f64_waiter *sender = waiter_pop(&channel->send_head, &channel->send_tail);
    if (sender) {
      channel->buffer[channel->tail] = sender->value; channel->tail = (channel->tail + 1) % channel->capacity; channel->count++;
      finish_waiter(channel, sender);
    }
    pthread_cond_broadcast(&channel->changed); pthread_mutex_unlock(&channel->mutex); return value;
  }
  tsnative_channel_f64_waiter *sender = waiter_pop(&channel->send_head, &channel->send_tail);
  if (sender) {
    value = sender->value; finish_waiter(channel, sender); pthread_mutex_unlock(&channel->mutex); return value;
  }
  tsnative_channel_f64_waiter *waiter = calloc(1, sizeof(*waiter));
  if (!waiter) abort();
  waiter->task = tsnative_scheduler_current_task(); waiter->out = &value; waiter->cooperative = 1;
  waiter_push(&channel->recv_head, &channel->recv_tail, waiter);
  pthread_mutex_unlock(&channel->mutex);
  wait_cooperatively(channel, waiter);
  free(waiter);
  return value;
}
