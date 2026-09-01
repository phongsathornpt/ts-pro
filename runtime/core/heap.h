#ifndef TSNATIVE_HEAP_H
#define TSNATIVE_HEAP_H

#include <stddef.h>

void *tsnative_heap_alloc(size_t size);
void tsnative_heap_shutdown(void);
size_t tsnative_heap_live_bytes(void);
size_t tsnative_heap_live_allocations(void);

void *tsnative_gc_enter(void **slots, size_t count);
void tsnative_gc_leave(void *frame);
void *tsnative_gc_root_register(void **slot);
void tsnative_gc_root_unregister(void *root);
void tsnative_gc_safepoint(void);
void tsnative_gc_collect(void);
size_t tsnative_gc_collections(void);

#endif
