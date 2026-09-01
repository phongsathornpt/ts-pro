#include "heap.h"

#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

typedef struct {
  uint64_t len;
  char data[];
} tsnative_string;

typedef enum {
  TSNATIVE_JS_NUMBER = 1,
  TSNATIVE_JS_STRING = 2,
} tsnative_js_tag;

typedef struct {
  uint32_t tag;
  uint32_t reserved;
  union {
    double number;
    void *ref;
  } payload;
} tsnative_jsvalue;
extern void *tsnative_string_new(const char *data, uint64_t len);
extern void *tsnative_string_concat(void *left, void *right);

static tsnative_jsvalue *new_value(uint32_t tag) {
  tsnative_jsvalue *value = tsnative_heap_alloc(sizeof(*value));
  value->tag = tag;
  value->reserved = 0;
  return value;
}

void *tsnative_jsvalue_box_f64(double number) {
  tsnative_jsvalue *value = new_value(TSNATIVE_JS_NUMBER);
  value->payload.number = number;
  return value;
}

void *tsnative_jsvalue_box_string(void *string) {
  tsnative_jsvalue *value = new_value(TSNATIVE_JS_STRING);
  value->payload.ref = string;
  return value;
}
static void *number_to_string(double number) {
  char buffer[64];
  int length = snprintf(buffer, sizeof(buffer), "%.17g", number);
  if (length < 0 || (size_t)length >= sizeof(buffer)) abort();
  return tsnative_string_new(buffer, (uint64_t)length);
}

static void *to_string(tsnative_jsvalue *value) {
  if (!value) abort();
  if (value->tag == TSNATIVE_JS_STRING) return value->payload.ref;
  if (value->tag == TSNATIVE_JS_NUMBER) return number_to_string(value->payload.number);
  abort();
}

void *tsnative_jsvalue_add(void *left_raw, void *right_raw) {
  tsnative_jsvalue *left = left_raw;
  tsnative_jsvalue *right = right_raw;
  if (!left || !right) abort();
  if (left->tag == TSNATIVE_JS_NUMBER && right->tag == TSNATIVE_JS_NUMBER) {
    return tsnative_jsvalue_box_f64(left->payload.number + right->payload.number);
  }
  void *left_string = to_string(left);
  void *right_string = to_string(right);
  void *combined = tsnative_string_concat(left_string, right_string);
  return tsnative_jsvalue_box_string(combined);
}

void tsnative_console_log_jsvalue(void *raw) {
  tsnative_jsvalue *value = raw;
  if (!value) abort();
  if (value->tag == TSNATIVE_JS_NUMBER) {
    printf("%.17g\n", value->payload.number);
    return;
  }
  if (value->tag == TSNATIVE_JS_STRING) {
    tsnative_string *string = value->payload.ref;
    if (string && string->len) fwrite(string->data, 1, string->len, stdout);
    fputc('\n', stdout);
    return;
  }
  abort();
}
