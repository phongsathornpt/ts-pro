async function dynamicAsyncMultiply(value: any): Promise<number> {
  const result = value * 7;
  sleep(1);
  return result;
}

console.log(join(dynamicAsyncMultiply("6")));
