async function reassignedPromiseNumber(): Promise<number> {
  let current: Promise<number> = Promise.resolve(10);
  current = current;
  const original: Promise<number> = current;
  current = Promise.resolve(20);
  const first: number = await original;
  const second: number = await current;
  return first + second;
}

async function reassignedPromiseString(): Promise<string> {
  let current: Promise<string> = Promise.resolve("old-root");
  const original: Promise<string> = current;
  current = Promise.resolve("new-root");
  current = Promise.resolve(current);
  const first: string = await original;
  const second: string = await current;
  return first + ":" + second;
}

console.log(join(reassignedPromiseNumber()));
console.log(join(reassignedPromiseString()));
