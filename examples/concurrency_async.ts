async function asyncLeaf(base: number): Promise<number> {
  sleep(1);
  return base + 2;
}

async function asyncParent(): Promise<number> {
  const value = await asyncLeaf(40);
  return value;
}

console.log(join(asyncParent()));
