async function finallyReturnOverride(): Promise<number> {
  try {
    return 40;
  } catch (error: any) {
    return 1;
  } finally {
    return 99;
  }
}

async function finallyThrowOverride(): Promise<number> {
  try {
    return 40;
  } catch (error: any) {
    return 1;
  } finally {
    throw "finally-override";
  }
}
async function catchFinallyOverride(): Promise<number> {
  try {
    return await finallyThrowOverride();
  } catch (error: any) {
    console.log(error);
    return 42;
  }
}

console.log(join(finallyReturnOverride()));
console.log(join(catchFinallyOverride()));
