async function recoverLocalThrow(): Promise<any> {
  try {
    throw "caught-local";
  } catch (error: any) {
    return error;
  }
}

console.log(join(recoverLocalThrow()));
