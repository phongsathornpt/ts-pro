async function runStandardPromiseLike(): Promise<void> {
  const rawPromiseLike: any = {
    then: function (resolve: (value: number) => void, reject: (reason: any) => void): void {
      resolve(42);
    },
  };
  const standardPromiseLike: PromiseLike<number> = rawPromiseLike;
  console.log(await Promise.resolve(standardPromiseLike));
}

runStandardPromiseLike();
