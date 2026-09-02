class DispatchBase {
  constructor(public value: number) {}

  score(): number {
    return this.value;
  }
}

class DispatchDerived extends DispatchBase {
  constructor(value: number) {
    super(value);
  }

  override score(): number {
    return this.value + 2;
  }
}

const item: DispatchBase = new DispatchDerived(40);
console.log(item.score());
