async function rejectedFinallyCompletionLeaf(): Promise<number> {
  sleep(1);
  throw "rethrow-finally";
}

async function returnThroughFinally(): Promise<number> {
  let value: number = 40;
  try {
    return value;
  } catch (error: any) {
    return 0;
  } finally {
    value = 99;
    console.log("finally-return");
  }
}

async function rethrowThroughFinally(): Promise<number> {
  try {
    return await rejectedFinallyCompletionLeaf();
  } catch (error: any) {
    throw error;
  } finally {
    console.log("finally-rethrow");
  }
}
async function catchRethrowFinally(): Promise<number> {
  try {
    return await rethrowThroughFinally();
  } catch (error: any) {
    console.log(error);
    return 42;
  }
}

console.log(join(returnThroughFinally()));
console.log(join(catchRethrowFinally()));
