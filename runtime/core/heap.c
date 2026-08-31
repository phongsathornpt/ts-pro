#include "heap.h"

#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>

typedef struct tsnative_heap_block {
  struct tsnative_heap_block *next;
  size_t size;
  uint8_t marked;
} tsnative_heap_block;

typedef struct tsnative_gc_frame {
  struct tsnative_gc_frame *prev;
  void **slots;
  size_t count;
} tsnative_gc_frame;

static tsnative_heap_block *tsnative_heap_head;
static tsnative_gc_frame *tsnative_gc_roots;
static size_t tsnative_heap_bytes;
static size_t tsnative_heap_allocations;
static size_t tsnative_collection_count;
static size_t tsnative_gc_threshold = 64 * 1024;

static void tsnative_gc_mark_candidate(void *candidate);

void *tsnative_heap_alloc(size_t size) {
  if (size == 0) size = 1;
  tsnative_heap_block *block = malloc(sizeof(*block) + size);
  if (!block) {
    fputs("tsnative: heap allocation failed\n", stderr);
    abort();
  }
  block->next = tsnative_heap_head;
  block->size = size;
  block->marked = 0;
  tsnative_heap_head = block;
  tsnative_heap_bytes += size;
  tsnative_heap_allocations++;
  return block + 1;
}

void *tsnative_gc_enter(void **slots, size_t count) {
  tsnative_gc_frame *frame = malloc(sizeof(*frame));
  if (!frame) abort();
  frame->prev = tsnative_gc_roots;
  frame->slots = slots;
  frame->count = count;
  tsnative_gc_roots = frame;
  return frame;
}

void tsnative_gc_leave(void *raw) {
  tsnative_gc_frame *frame = raw;
  if (!frame || tsnative_gc_roots != frame) {
    fputs("tsnative: invalid GC root frame discipline\n", stderr);
    abort();
  }
  tsnative_gc_roots = frame->prev;
  free(frame);
}

static tsnative_heap_block *tsnative_find_block(void *candidate) {
  for (tsnative_heap_block *block = tsnative_heap_head; block; block = block->next) {
    if ((void *)(block + 1) == candidate) return block;
  }
  return NULL;
}

static void tsnative_gc_mark_candidate(void *candidate) {
  if (!candidate) return;
  tsnative_heap_block *block = tsnative_find_block(candidate);
  if (!block || block->marked) return;
  block->marked = 1;
  uintptr_t *words = (uintptr_t *)(block + 1);
  size_t count = block->size / sizeof(uintptr_t);
  for (size_t i = 0; i < count; i++) {
    tsnative_gc_mark_candidate((void *)words[i]);
  }
}

void tsnative_gc_collect(void) {
  for (tsnative_heap_block *block = tsnative_heap_head; block; block = block->next) {
    block->marked = 0;
  }
  for (tsnative_gc_frame *frame = tsnative_gc_roots; frame; frame = frame->prev) {
    for (size_t i = 0; i < frame->count; i++) {
      tsnative_gc_mark_candidate(frame->slots[i]);
    }
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

void tsnative_gc_safepoint(void) {
  if (tsnative_heap_bytes >= tsnative_gc_threshold) {
    tsnative_gc_collect();
  }
}

void tsnative_heap_shutdown(void) {
  tsnative_heap_block *block = tsnative_heap_head;
  while (block) {
    tsnative_heap_block *next = block->next;
    free(block);
    block = next;
  }
  while (tsnative_gc_roots) {
    tsnative_gc_frame *prev = tsnative_gc_roots->prev;
    free(tsnative_gc_roots);
    tsnative_gc_roots = prev;
  }
  tsnative_heap_head = NULL;
  tsnative_heap_bytes = 0;
  tsnative_heap_allocations = 0;
}

size_t tsnative_heap_live_bytes(void) {
  return tsnative_heap_bytes;
}

size_t tsnative_heap_live_allocations(void) {
  return tsnative_heap_allocations;
}

size_t tsnative_gc_collections(void) {
  return tsnative_collection_count;
}
