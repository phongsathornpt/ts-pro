#include "heap.h"

#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>

typedef struct tsnative_heap_block {
  struct tsnative_heap_block *next;
  size_t size;
} tsnative_heap_block;

static tsnative_heap_block *tsnative_heap_head;
static size_t tsnative_heap_bytes;
static size_t tsnative_heap_allocations;

void *tsnative_heap_alloc(size_t size) {
  if (size == 0) size = 1;
  tsnative_heap_block *block = malloc(sizeof(*block) + size);
  if (!block) {
    fputs("tsnative: heap allocation failed\n", stderr);
    abort();
  }
  block->next = tsnative_heap_head;
  block->size = size;
  tsnative_heap_head = block;
  tsnative_heap_bytes += size;
  tsnative_heap_allocations++;
  return block + 1;
}

void tsnative_heap_shutdown(void) {
  tsnative_heap_block *block = tsnative_heap_head;
  while (block) {
    tsnative_heap_block *next = block->next;
    free(block);
    block = next;
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
