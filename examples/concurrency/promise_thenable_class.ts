class BaseThenable {
  value: number;

  constructor(value: number) {
    this.value = value;
  }

  then(resolve: (value: number) => void, reject: (reason: any) => void): void {
    resolve(this.value);
  }
}

class DerivedThenable extends BaseThenable {
  constructor(value: number) {
    super(value);
  }

  override then(resolve: (value: number) => void, reject: (reason: any) => void): void {
    resolve(this.value + 2);
  }
}

async function runClassThenable(): Promise<void> {
  const thenable: BaseThenable = new DerivedThenable(40);
  console.log(await Promise.resolve(thenable));
}

runClassThenable();
