async function rejectedFinallyLeaf(): Promise<number> {
  sleep(1);
  throw "caught-finally";
}

async function recoverWithFinally(): Promise<number> {
  let value: number = 0;
  try {
    value = await rejectedFinallyLeaf();
  } catch (error: any) {
    console.log(error);
    value = 1;
  } finally {
    value = 2;
  }
  return value;
}

console.log(join(recoverWithFinally()));
