function makeAdder(base: number): (x: number) => number {
  const add = (x: number): number => base + x;
  return add;
}

console.log(makeAdder(5)(7));
