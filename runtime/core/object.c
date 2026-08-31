#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>

void *tsnative_object_alloc(size_t size) {
    if (size == 0) {
        size = 1;
    }
    void *object = malloc(size);
    if (object == NULL) {
        fputs("tsnative: object allocation failed\n", stderr);
        abort();
    }
    return object;
}
