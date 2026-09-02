class NativeBase {
  constructor(public x: number) {}

  value(): number {
    return this.x;
  }
}

class NativeDerived extends NativeBase {
  y: number = 2;

  constructor(x: number) {
    super(x);
  }

  sum(): number {
    return this.x + this.y;
  }
}

console.log(new NativeDerived(40).sum());
console.log(new NativeDerived(40).value());
