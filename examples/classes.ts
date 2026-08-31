class NativePoint {
  constructor(public x: number, public y: number) {}

  sum(): number {
    return this.x + this.y;
  }
}

console.log(new NativePoint(3, 4).sum());
