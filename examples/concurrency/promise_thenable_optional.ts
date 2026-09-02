interface OptionalThenable {
  base: number;
  then: (
    resolve: ((value: number) => void) | undefined | null,
    reject: ((reason: any) => void) | undefined | null,
  ) => OptionalThenable;
}

async function runOptionalThenable(): Promise<void> {
  const raw: any = {
    base: 40,
    then: function (
      resolve: (value: number) => void,
      reject: (reason: any) => void,
    ): void {
      resolve(42);
    },
  };
  const thenable: OptionalThenable = raw;
  console.log(await Promise.resolve(thenable));
}

runOptionalThenable();
