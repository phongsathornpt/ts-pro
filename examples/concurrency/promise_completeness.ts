interface CustomThenable {
  val: number;
  then: (
    this: CustomThenable,
    onFulfilled: (value: number) => number,
    onRejected: (reason: string) => string,
  ) => string;
}

async function testPromiseLike(): Promise<void> {
  const pl: PromiseLike<number> = Promise.resolve(42);
  const resolved = await pl;
  console.log(resolved);
}

async function testHeterogeneousTuple(): Promise<void> {
  const pNum = Promise.resolve(100);
  const pStr = Promise.resolve("hello");
  const pBool = Promise.resolve(true);
  const [n, s, b] = await Promise.all([pNum, pStr, pBool]);
  console.log(n);
  console.log(s);
  console.log(b);
}

async function testArrayVariable(): Promise<void> {
  const p1 = Promise.resolve(10);
  const p2 = Promise.resolve(20);
  const arr = [p1, p2];
  const allRes = await Promise.all(arr);
  console.log(allRes[0]);
  console.log(allRes[1]);

  const p3 = Promise.resolve(30);
  const raceArr = [p3];
  const raceRes = await Promise.race(raceArr);
  console.log(raceRes);
}

async function testThenable(): Promise<void> {
  const thenable: CustomThenable = {
    val: 50,
    then: function (
      this: CustomThenable,
      onFulfilled: (value: number) => number,
      onRejected: (reason: string) => string,
    ): string {
      onFulfilled(this.val);
      return "done";
    },
  };
  const res = await Promise.resolve(thenable);
  console.log(res);
}

async function testThenableInPromiseAll(): Promise<void> {
  const thenable: CustomThenable = {
    val: 60,
    then: function (
      this: CustomThenable,
      onFulfilled: (value: number) => number,
      onRejected: (reason: string) => string,
    ): string {
      onFulfilled(this.val);
      return "done";
    },
  };
  const [res] = await Promise.all([Promise.resolve(thenable)]);
  console.log(res);
}

async function main(): Promise<void> {
  await testPromiseLike();
  await testHeterogeneousTuple();
  await testArrayVariable();
  await testThenable();
  await testThenableInPromiseAll();
}

main();
