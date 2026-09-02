async function adoptedPromiseNumber(): Promise<number> {
  const original: Promise<number> = Promise.resolve(21);
  const adopted: Promise<number> = Promise.resolve(original);
  const alias: Promise<number> = original;
  const first: number = await adopted;
  const second: number = await alias;
  return first + second;
}

async function adoptedPromiseString(): Promise<string> {
  const original: Promise<string> = Promise.resolve("adopt-root");
  const adopted: Promise<string> = Promise.resolve(original);
  const first: string = await original;
  const second: string = await adopted;
  return first + ":" + second;
}

console.log(join(adoptedPromiseNumber()));
console.log(join(adoptedPromiseString()));
