class NativeBox {
  value: number = 2;

  constructor(public delta: number) {
    console.log(this.value);
    this.value = this.value + delta;
  }

  get(): number {
    return this.value;
  }
}

const box = new NativeBox(3);
console.log(box.get());
