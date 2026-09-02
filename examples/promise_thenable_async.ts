interface AsyncNumberThenable {
  base: number;
  then: (
    this: AsyncNumberThenable,
    resolve: (value: number) => void,
    reject: (reason: any) => void,
  ) => void;
}

const delayedThenable: AsyncNumberThenable = {
  base: 40,
  then: function (this: AsyncNumberThenable, resolve: (value: number) => void, reject: (reason: any) => void): void {
    const value = this.base + 2;
    spawn((): void => {
      sleep(5);
      resolve(value);
      reject("late-async-reject");
    });
  },
};

const delayedPromise: Promise<number> = Promise.resolve(delayedThenable);
console.log(join(delayedPromise));

async function delayedRejectResult(): Promise<number> {
  const rejected: AsyncNumberThenable = {
    base: 0,
    then: function (this: AsyncNumberThenable, resolve: (value: number) => void, reject: (reason: any) => void): void {
      spawn((): void => {
        sleep(5);
        reject("async-thenable-reject");
        resolve(99);
      });
    },
  };
  try {
    return await Promise.resolve(rejected);
  } catch (error: any) {
    console.log(error);
    return 7;
  }
}

console.log(join(delayedRejectResult()));
