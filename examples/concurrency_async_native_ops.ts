function asyncDoubleValue(value: number): number {
  return value * 2;
}

function asyncApply(fn: () => number): number {
  return fn();
}

async function asyncNativeOps(base: number): Promise<number> {
  const values = [base, base + 1];
  const point = { value: values[0]! };
  point.value = point.value + 1;
  const fn = (): number => point.value + 1;
  const direct = fn();
  const applied = asyncApply(fn);
  const doubled = asyncDoubleValue(point.value);
  values[1] = doubled + applied + direct;
  sleep(1);
  return values[1]!;
}

console.log(join(asyncNativeOps(5)));
