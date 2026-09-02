class VirtualBase {
  constructor(public value: number) {}

  score(): number {
    return this.value;
  }
}

class VirtualDerived extends VirtualBase {
  constructor(value: number) {
    super(value);
  }

  override score(): number {
    return this.value + 2;
  }
}

function readScore(item: VirtualBase): number {
  return item.score();
}

console.log(readScore(new VirtualDerived(40)));
