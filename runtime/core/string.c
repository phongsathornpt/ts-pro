#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

typedef struct {
  uint64_t len;
  char data[];
} tsnative_string;

void *tsnative_string_new(const char *data, uint64_t len) {
  tsnative_string *value = malloc(sizeof(*value) + len);
  if (!value) abort();
  value->len = len;
  if (len != 0 && data) memcpy(value->data, data, len);
  return value;
}

void *tsnative_string_concat(void *left_raw, void *right_raw) {
  tsnative_string *left = left_raw;
  tsnative_string *right = right_raw;
  uint64_t left_len = left ? left->len : 0;
  uint64_t right_len = right ? right->len : 0;
  tsnative_string *result = tsnative_string_new(NULL, left_len + right_len);
  if (left_len) memcpy(result->data, left->data, left_len);
  if (right_len) memcpy(result->data + left_len, right->data, right_len);
  return result;
}

void tsnative_console_log_string(void *raw) {
  tsnative_string *value = raw;
  if (value && value->len) fwrite(value->data, 1, value->len, stdout);
  fputc('\n', stdout);
}
