async function rejectedAwaitLeaf(): Promise<number> {
  sleep(1);
  throw "caught-await";
}

async function recoverAwaitedRejection(): Promise<any> {
  try {
    return await rejectedAwaitLeaf();
  } catch (error: any) {
    return error;
  }
}

console.log(join(recoverAwaitedRejection()));
