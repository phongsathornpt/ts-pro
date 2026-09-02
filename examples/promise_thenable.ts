interface NumberThenable {
  base: number;
  then: (
    this: NumberThenable,
    resolve: (value: number) => void,
    reject: (reason: any) => void,
  ) => void;
}

async function runThenables(): Promise<void> {
  const resolved: NumberThenable = {
    base: 40,
    then: function (this: NumberThenable, resolve: (value: number) => void, reject: (reason: any) => void): void {
      resolve(this.base + 2);
      reject("late-reject");
    },
  };
  console.log(await Promise.resolve(resolved));

  const rejected: NumberThenable = {
    base: 0,
    then: function (this: NumberThenable, resolve: (value: number) => void, reject: (reason: any) => void): void {
      reject("thenable-reject");
      resolve(99);
    },
  };
  try {
    console.log(await Promise.resolve(rejected));
  } catch (error: any) {
    console.log(error);
    console.log(7);
  }
}

runThenables();
