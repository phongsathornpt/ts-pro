#ifndef TSNATIVE_HEAP_H
#define TSNATIVE_HEAP_H

#include <stddef.h>

void *tsnative_heap_alloc(size_t size);
void tsnative_heap_shutdown(void);
size_t tsnative_heap_live_bytes(void);
size_t tsnative_heap_live_allocations(void);

#endif
