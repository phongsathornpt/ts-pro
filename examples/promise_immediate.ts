async function resolvedNumber(): Promise<number> {
  return await Promise.resolve(41);
}

async function recoveredImmediateReject(): Promise<number> {
  try {
    return await Promise.reject<number>("promise-reject");
  } catch (error: any) {
    console.log(error);
    return 42;
  }
}

async function resolvedString(): Promise<string> {
  return await Promise.resolve("rooted-promise");
}

console.log(join(resolvedNumber()));
console.log(join(recoveredImmediateReject()));
console.log(join(resolvedString()));
