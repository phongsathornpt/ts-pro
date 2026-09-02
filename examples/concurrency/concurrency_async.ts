async function asyncLeaf(base: number): Promise<number> {
  sleep(1);
  return base;
}

async function asyncParent(): Promise<number> {
  const value = await asyncLeaf(42);
  return value;
}

console.log(join(asyncParent()));
