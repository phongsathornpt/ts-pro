class StandardPromiseLike implements PromiseLike<number> {
  value: number;

  constructor(value: number) {
    this.value = value;
  }

  then(
    onfulfilled?: ((value: number) => any) | null,
    onrejected?: ((reason: any) => any) | null,
  ): any {
    if (onfulfilled === undefined) {
      return this;
    }
    if (onfulfilled === null) {
      return this;
    }
    onfulfilled(this.value + 2);
    return this;
  }
}

async function runStandardPromiseLikeClass(): Promise<void> {
  const value: Promise<number> = Promise.resolve(new StandardPromiseLike(40));
  console.log(await value);
}

runStandardPromiseLikeClass();
