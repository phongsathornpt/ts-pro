function mutateNumbers(xs: number[]): number {
  xs[1] = 10;
  return xs[0]! + xs[1]! + xs[2]!;
}

console.log(mutateNumbers([1, 2, 3]));
