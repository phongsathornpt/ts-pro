function consumeAny(value: any): number {
  return 1;
}

function increment(value: number): number {
  return value + 1;
}

console.log(consumeAny({ value: 42 }));
console.log(consumeAny([1, 2]));
console.log(consumeAny(increment));
