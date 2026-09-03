export function multiply(a: number, b: number): number {
  return a * b;
}

export function power(base: number, exp: number): number {
  let res = 1;
  for (let i = 0; i < exp; i++) {
    res = res * base;
  }
  return res;
}

export class Counter {
  count: number;
  constructor(initial: number) {
    this.count = initial;
  }
  increment(): number {
    this.count = this.count + 1;
    return this.count;
  }
  value(): number {
    return this.count;
  }
}
