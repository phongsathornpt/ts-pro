class DynamicThisCounter {
  value: number;

  constructor(value: number) {
    this.value = value;
  }

  add(delta: number): number {
    this.value = this.value + delta;
    return this.value;
  }
}

const dynamicCounter: any = new DynamicThisCounter(40);
console.log(dynamicCounter.add(2));
console.log(dynamicCounter.add(8));
