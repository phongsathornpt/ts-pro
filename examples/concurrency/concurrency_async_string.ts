async function asyncStringLeaf(value: string): Promise<string> {
  sleep(1);
  return value + "-done";
}

async function asyncStringParent(value: string): Promise<string> {
  const result = await asyncStringLeaf(value);
  sleep(1);
  return result;
}

console.log(join(asyncStringParent("async-string")));
