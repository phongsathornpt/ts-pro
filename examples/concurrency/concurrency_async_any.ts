async function asyncAnyLeaf(value: any): Promise<any> {
  sleep(1);
  return value;
}

async function asyncAnyParent(value: any): Promise<any> {
  return await asyncAnyLeaf(value);
}

console.log(join(asyncAnyParent("async-any")));
