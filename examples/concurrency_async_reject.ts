async function rejectedLeaf(): Promise<number> {
  sleep(1);
  throw "async-boom";
}

async function rejectedParent(): Promise<number> {
  return await rejectedLeaf();
}

join(rejectedParent());
console.log(99);
