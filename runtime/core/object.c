#include "heap.h"

#include <stddef.h>

void *tsnative_object_alloc(size_t size) {
    return tsnative_heap_alloc(size);
}
