#include <math.h>
#include <stdint.h>
#include <stdlib.h>

typedef struct {
  uint64_t len;
  double data[];
} tsnative_array_f64;

void *tsnative_array_f64_new(uint64_t len) {
  tsnative_array_f64 *array = malloc(sizeof(*array) + sizeof(double) * len);
  if (!array) abort();
  array->len = len;
  return array;
}

void tsnative_array_f64_set(void *raw, uint64_t index, double value) {
  tsnative_array_f64 *array = raw;
  if (!array || index >= array->len) abort();
  array->data[index] = value;
}

double tsnative_array_f64_len(void *raw) {
  tsnative_array_f64 *array = raw;
  return array ? (double)array->len : 0.0;
}

double tsnative_array_f64_get(void *raw, double index) {
  tsnative_array_f64 *array = raw;
  if (!array || !isfinite(index) || index < 0.0) return NAN;
  uint64_t i = (uint64_t)index;
  if ((double)i != index || i >= array->len) return NAN;
  return array->data[i];
}
