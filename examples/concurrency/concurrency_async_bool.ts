async function asyncBoolLeaf(value: boolean): Promise<boolean> {
  sleep(1);
  return value;
}

async function asyncBoolParent(value: boolean): Promise<boolean> {
  return await asyncBoolLeaf(value);
}

function boolScore(value: boolean): number {
  if (value) return 42;
  return 0;
}

console.log(boolScore(join(asyncBoolParent(true))));
