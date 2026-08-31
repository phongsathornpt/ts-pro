function makeTopLevelAdder(base: number): (x: number) => number {
  const add = (x: number): number => base + x;
  return add;
}

const add5 = makeTopLevelAdder(5);
let result = add5(7);
result = result + 1;
console.log(result);
