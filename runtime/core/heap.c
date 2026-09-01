#include "heap.h"

#include <pthread.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>

typedef struct tsnative_heap_block {
  struct tsnative_heap_block *next;
  size_t size;
  uint8_t marked;
} tsnative_heap_block;

typedef struct tsnative_gc_frame {
  struct tsnative_gc_frame *prev_local;
  struct tsnative_gc_frame *prev_global;
  struct tsnative_gc_frame *next_global;
  void **slots;
  size_t count;
  int persistent;
} tsnative_gc_frame;

static pthread_mutex_t tsnative_heap_mutex = PTHREAD_MUTEX_INITIALIZER;
static tsnative_heap_block *tsnative_heap_head;
static tsnative_gc_frame *tsnative_gc_roots;
static size_t tsnative_heap_bytes;
static size_t tsnative_heap_allocations;
static size_t tsnative_collection_count;
static size_t tsnative_gc_threshold = 64 * 1024;
static size_t tsnative_active_root_threads;
static size_t tsnative_gc_handoffs;
static _Thread_local tsnative_gc_frame *tsnative_local_roots;
static _Thread_local size_t tsnative_local_root_depth;

static void tsnative_gc_mark_candidate_locked(void *candidate);

static void tsnative_root_insert_locked(tsnative_gc_frame *frame) {
  frame->prev_global = NULL;
  frame->next_global = tsnative_gc_roots;
  if (tsnative_gc_roots) tsnative_gc_roots->prev_global = frame;
  tsnative_gc_roots = frame;
}

static void tsnative_root_remove_locked(tsnative_gc_frame *frame) {
  if (frame->prev_global) frame->prev_global->next_global = frame->next_global;
  else tsnative_gc_roots = frame->next_global;
  if (frame->next_global) frame->next_global->prev_global = frame->prev_global;
  frame->prev_global = frame->next_global = NULL;
}

void *tsnative_heap_alloc(size_t size) {
  if (size == 0) size = 1;
  tsnative_heap_block *block = malloc(sizeof(*block) + size);
  if (!block) {
    fputs("tsnative: heap allocation failed\n", stderr);
    abort();
  }
  block->size = size;
  block->marked = 0;
  pthread_mutex_lock(&tsnative_heap_mutex);
  block->next = tsnative_heap_head;
  tsnative_heap_head = block;
  tsnative_heap_bytes += size;
  tsnative_heap_allocations++;
  pthread_mutex_unlock(&tsnative_heap_mutex);
  return block + 1;
}

void *tsnative_gc_enter(void **slots, size_t count) {
  tsnative_gc_frame *frame = calloc(1, sizeof(*frame));
  if (!frame) abort();
  frame->slots = slots;
  frame->count = count;
  pthread_mutex_lock(&tsnative_heap_mutex);
  frame->prev_local = tsnative_local_roots;
  tsnative_local_roots = frame;
  if (tsnative_local_root_depth++ == 0) tsnative_active_root_threads++;
  tsnative_root_insert_locked(frame);
  pthread_mutex_unlock(&tsnative_heap_mutex);
  return frame;
}

void tsnative_gc_leave(void *raw) {
  tsnative_gc_frame *frame = raw;
  pthread_mutex_lock(&tsnative_heap_mutex);
  if (!frame || frame->persistent || tsnative_local_roots != frame || tsnative_local_root_depth == 0) {
    pthread_mutex_unlock(&tsnative_heap_mutex);
    fputs("tsnative: invalid GC root frame discipline\n", stderr);
    abort();
  }
  tsnative_local_roots = frame->prev_local;
  tsnative_local_root_depth--;
  if (tsnative_local_root_depth == 0) tsnative_active_root_threads--;
  tsnative_root_remove_locked(frame);
  pthread_mutex_unlock(&tsnative_heap_mutex);
  free(frame);
}

void *tsnative_gc_root_register(void **slot) {
  if (!slot) return NULL;
  tsnative_gc_frame *frame = calloc(1, sizeof(*frame));
  if (!frame) return NULL;
  frame->slots = slot;
  frame->count = 1;
  frame->persistent = 1;
  pthread_mutex_lock(&tsnative_heap_mutex);
  tsnative_root_insert_locked(frame);
  pthread_mutex_unlock(&tsnative_heap_mutex);
  return frame;
}

void tsnative_gc_root_unregister(void *raw) {
  tsnative_gc_frame *frame = raw;
  if (!frame) return;
  pthread_mutex_lock(&tsnative_heap_mutex);
  if (!frame->persistent) {
    pthread_mutex_unlock(&tsnative_heap_mutex);
    fputs("tsnative: invalid persistent GC root\n", stderr);
    abort();
  }
  tsnative_root_remove_locked(frame);
  pthread_mutex_unlock(&tsnative_heap_mutex);
  free(frame);
}

void tsnative_gc_handoff_begin(void) {
  pthread_mutex_lock(&tsnative_heap_mutex);
  tsnative_gc_handoffs++;
  pthread_mutex_unlock(&tsnative_heap_mutex);
}

void tsnative_gc_handoff_end(void) {
  pthread_mutex_lock(&tsnative_heap_mutex);
  if (tsnative_gc_handoffs == 0) {
    pthread_mutex_unlock(&tsnative_heap_mutex);
    fputs("tsnative: invalid GC handoff discipline\n", stderr);
    abort();
  }
  tsnative_gc_handoffs--;
  pthread_mutex_unlock(&tsnative_heap_mutex);
}

static tsnative_heap_block *tsnative_find_block_locked(void *candidate) {
  for (tsnative_heap_block *block = tsnative_heap_head; block; block = block->next) {
    if ((void *)(block + 1) == candidate) return block;
  }
  return NULL;
}

static void tsnative_gc_mark_candidate_locked(void *candidate) {
  if (!candidate) return;
  tsnative_heap_block *block = tsnative_find_block_locked(candidate);
  if (!block || block->marked) return;
  block->marked = 1;
  uintptr_t *words = (uintptr_t *)(block + 1);
  size_t count = block->size / sizeof(uintptr_t);
  for (size_t i = 0; i < count; i++) {
    tsnative_gc_mark_candidate_locked((void *)words[i]);
  }
}

static void tsnative_gc_collect_locked(void) {
  for (tsnative_heap_block *block = tsnative_heap_head; block; block = block->next) block->marked = 0;
  for (tsnative_gc_frame *frame = tsnative_gc_roots; frame; frame = frame->next_global) {
    for (size_t i = 0; i < frame->count; i++) tsnative_gc_mark_candidate_locked(frame->slots[i]);
  }
  tsnative_heap_block **link = &tsnative_heap_head;
  while (*link) {
    tsnative_heap_block *block = *link;
    if (block->marked) {
      link = &block->next;
      continue;
    }
    *link = block->next;
    tsnative_heap_bytes -= block->size;
    tsnative_heap_allocations--;
    free(block);
  }
  tsnative_collection_count++;
  size_t next = tsnative_heap_bytes * 2;
  tsnative_gc_threshold = next > 64 * 1024 ? next : 64 * 1024;
}

void tsnative_gc_collect(void) {
  pthread_mutex_lock(&tsnative_heap_mutex);
  if (tsnative_active_root_threads <= 1 && tsnative_gc_handoffs == 0) tsnative_gc_collect_locked();
  pthread_mutex_unlock(&tsnative_heap_mutex);
}

void tsnative_gc_safepoint(void) {
  pthread_mutex_lock(&tsnative_heap_mutex);
  if (tsnative_heap_bytes >= tsnative_gc_threshold && tsnative_active_root_threads <= 1 && tsnative_gc_handoffs == 0) {
    tsnative_gc_collect_locked();
  }
  pthread_mutex_unlock(&tsnative_heap_mutex);
}

void tsnative_heap_shutdown(void) {
  pthread_mutex_lock(&tsnative_heap_mutex);
  tsnative_heap_block *block = tsnative_heap_head;
  while (block) {
    tsnative_heap_block *next = block->next;
    free(block);
    block = next;
  }
  while (tsnative_gc_roots) {
    tsnative_gc_frame *next = tsnative_gc_roots->next_global;
    free(tsnative_gc_roots);
    tsnative_gc_roots = next;
  }
  tsnative_heap_head = NULL;
  tsnative_heap_bytes = 0;
  tsnative_heap_allocations = 0;
  tsnative_gc_threshold = 64 * 1024;
  tsnative_gc_handoffs = 0;
  pthread_mutex_unlock(&tsnative_heap_mutex);
}

size_t tsnative_heap_live_bytes(void) {
  pthread_mutex_lock(&tsnative_heap_mutex);
  size_t value = tsnative_heap_bytes;
  pthread_mutex_unlock(&tsnative_heap_mutex);
  return value;
}

size_t tsnative_heap_live_allocations(void) {
  pthread_mutex_lock(&tsnative_heap_mutex);
  size_t value = tsnative_heap_allocations;
  pthread_mutex_unlock(&tsnative_heap_mutex);
  return value;
}

size_t tsnative_gc_collections(void) {
  pthread_mutex_lock(&tsnative_heap_mutex);
  size_t value = tsnative_collection_count;
  pthread_mutex_unlock(&tsnative_heap_mutex);
  return value;
}
