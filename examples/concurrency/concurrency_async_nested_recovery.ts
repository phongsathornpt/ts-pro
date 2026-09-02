async function nestedRejectedLeaf(): Promise<number> {
  sleep(1);
  throw "leaf-rejection";
}

async function nestedRecover(): Promise<number> {
  try {
    try {
      return await nestedRejectedLeaf();
    } catch (error: any) {
      console.log(error);
      throw "middle-rethrow";
    } finally {
      console.log("inner-finally");
    }
  } catch (error: any) {
    console.log(error);
    return 7;
  } finally {
    console.log("outer-finally");
  }
}

console.log(join(nestedRecover()));
