export function sumArray(xs: number[]): number {
  let total: number = 0;
  for (let i: number = 0; i < xs.length; i++) {
    total = total + xs[i]!;
  }
  return total;
}

console.log(sumArray([1, 2, 3, 4, 5]));
