async function repeatResolvedNumber(): Promise<number> {
  const promise: Promise<number> = Promise.resolve(21);
  const first: number = await promise;
  const second: number = await promise;
  return first + second;
}

async function repeatResolvedString(): Promise<string> {
  const promise: Promise<string> = Promise.resolve("repeat-root");
  const first: string = await promise;
  const second: string = await promise;
  return first + ":" + second;
}

console.log(join(repeatResolvedNumber()));
console.log(join(repeatResolvedString()));
