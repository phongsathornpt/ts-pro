async function asyncMathLeaf(base: number): Promise<number> {
  sleep(1);
  return base + 2;
}

async function asyncMathParent(): Promise<number> {
  const value = await asyncMathLeaf(40);
  const doubled = value * 2;
  sleep(1);
  return doubled + 1;
}

async function asyncCompareParent(): Promise<boolean> {
  const value = await asyncMathLeaf(40);
  const ok = value > 41;
  sleep(1);
  return ok;
}

function asyncBoolScore(value: boolean): number {
  if (value) return 42;
  return 0;
}

console.log(join(asyncMathParent()));
console.log(asyncBoolScore(join(asyncCompareParent())));
